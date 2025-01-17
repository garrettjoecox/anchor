package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const JSON_TEMPLATE = `{"gamesComplete":0,"onlineCount":0,"lastStatsHeartbeat":"","nextClientId":1,"banList":[],"clientSHAs":[]}`
const INACTIVITY_TIMEOUT = 5 * time.Minute
const HEARTBEAT = 30 * time.Second
const BANNABLE_INVALID_PACKET_LIMIT = 5

type Server struct {
	listener       net.Listener
	quietMode      bool
	onlineClients  map[uint64]*Client
	rooms          map[string]*Room
	gamesCompleted uint64
	nextClientId   uint64
	banList        []string
	clientSHAs     []string
	mu             sync.Mutex
}

func NewServer() *Server {
	return &Server{
		onlineClients:  make(map[uint64]*Client),
		quietMode:      false,
		rooms:          make(map[string]*Room),
		gamesCompleted: 0,
		nextClientId:   1,
		banList:        make([]string, 0),
		clientSHAs:     make([]string, 0),
	}
}

func (s *Server) Start(errChan chan error) {
	listener, err := net.Listen("tcp", ":43383")
	if err != nil {
		log.Fatal(err)
	}
	s.listener = listener

	go s.cleanupInactiveRooms(errChan)
	go s.heartbeat(errChan)
	s.parseStats(errChan)
	go s.statsHeartbeat(errChan)

	log.Println("Server running on :43383")

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Println("Error with listener:", err)
				break
			}
			log.Println("Error accepting connection:", err)
			conn.Close()
			continue
		}

		ip := s.getSHA(conn)

		s.mu.Lock()
		//If they are banned stop here
		if slices.Contains(s.banList, ip) {
			s.handleBannedConnection(conn)
			s.mu.Unlock()
			continue
		}

		if !slices.Contains(s.clientSHAs, ip) {
			s.clientSHAs = append(s.clientSHAs, ip)
		}
		s.mu.Unlock()

		go s.handleConnection(conn, errChan)
	}
}

func (s *Server) parseStats(errChan chan error) {
	defer func() {
		if r := recover(); r != nil {
			errChan <- fmt.Errorf("panic in parseStats: %v", r)
		}
	}()

	value, err := os.ReadFile("stats.json")
	if err != nil {
		log.Println("Error reading stats.json file:", err)
	}

	//input values into their repective fields of the server
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gamesCompleted = uint64(gjson.Get(string(value), "gamesComplete").Int())
	s.nextClientId = uint64(gjson.Get(string(value), "nextClientId").Int())

	array := gjson.Parse(string(value)).Get("clientSHAs").Array()
	for _, value := range array {
		s.clientSHAs = append(s.clientSHAs, value.Str)
	}

	array = gjson.Parse(string(value)).Get("banList").Array()
	for _, value := range array {
		s.banList = append(s.banList, value.Str)
	}

}

func (s *Server) saveStats() {
	s.mu.Lock()
	//list of stats to save. Should all be in the JSON_TEMPLATE const
	value, _ := sjson.Set(JSON_TEMPLATE, "gamesComplete", s.gamesCompleted)
	value, _ = sjson.Set(value, "nextClientId", s.nextClientId)
	value, _ = sjson.Set(value, "onlineCount", len(s.onlineClients))
	value, _ = sjson.Set(value, "lastStatsHeartbeat", time.Now())
	value, _ = sjson.Set(value, "clientSHAs", s.clientSHAs)
	value, _ = sjson.Set(value, "banList", s.banList)
	s.mu.Unlock()

	err := os.WriteFile("./stats.json", []byte(value), 0644)

	if err != nil {
		log.Println("Error writing json to file: ", err)
	}
}

func (s *Server) cleanupInactiveRooms(errChan chan error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer func() {
		if r := recover(); r != nil {
			errChan <- fmt.Errorf("panic in cleanupInactiveRooms: %v", r)
		}
	}()

	for range ticker.C {
		s.mu.Lock()
		for id, room := range s.rooms {
			lastActivity := room.GetLastActivity()
			if time.Since(lastActivity) > INACTIVITY_TIMEOUT {
				log.Println("Room", id, "has been inactive for too long, deleting it")
				delete(s.rooms, id)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) statsHeartbeat(errChan chan error) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	defer func() {
		if r := recover(); r != nil {
			errChan <- fmt.Errorf("panic in statsHeartbeat: %v", r)
		}
	}()

	for range ticker.C {
		s.saveStats()
	}
}

func (s *Server) heartbeat(errChan chan error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer func() {
		if r := recover(); r != nil {
			errChan <- fmt.Errorf("panic in heartbeat: %v", r)
		}
	}()

	for range ticker.C {
		if !s.quietMode {
			log.Println("Clients Online & Threads Running", len(s.onlineClients), runtime.NumGoroutine())
		}

		s.mu.Lock()
		for _, client := range s.onlineClients {
			client.mu.Lock()
			if time.Since(client.lastActivity) > HEARTBEAT {
				go client.sendPacket(`{"type":"HEARTBEAT","quiet":true}`)
			}
			client.mu.Unlock()
		}
		s.mu.Unlock()
	}
}

func (s *Server) handleConnection(conn net.Conn, errChan chan error) {
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			errChan <- fmt.Errorf("panic in handleConnection: %v", r)
		}
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Split(splitNullByte)

	invalidCount := 0

	var client *Client

	for scanner.Scan() {
		//for those pesky bots out there ;)
		if invalidCount >= BANNABLE_INVALID_PACKET_LIMIT {
			s.banIP(strings.Split(conn.RemoteAddr().String(), ":")[0])
			s.handleBannedConnection(conn)
		}
		packet := scanner.Text()

		if !gjson.Valid(packet) {
			log.Printf("Invalid JSON packet: %s\n", packet)
			invalidCount++
			continue
		}

		packetTypeWrapped := gjson.Get(packet, "type")
		if !packetTypeWrapped.Exists() {
			log.Println("Packet missing type")
			invalidCount++
			continue
		}

		packetType := packetTypeWrapped.String()

		// Health check
		if packetType == "STATS" {
			outgoingPacket, _ := sjson.Set(`{"type":"STATS"}`, "uniquePlayers", s.nextClientId)
			outgoingPacket, _ = sjson.Set(outgoingPacket, "gamesCompleted", s.gamesCompleted)
			outgoingPacket, _ = sjson.Set(outgoingPacket, "online", len(s.onlineClients))
			conn.Write(append([]byte(outgoingPacket), 0))
			continue
		}

		if client == nil {
			if packetType != "HANDSHAKE" {
				log.Println("Client must handshake first")
				continue
			}

			client = s.findOrCreateClient(packet, conn)
			log.Printf("Client %v Connected\n", client.id)
			client.room.broadcastAllClientState()
			client.sendRoomState()
		} else {
			if invalidCount > 0 {
				invalidCount = 0
			}
			client.handlePacket(packet)
		}
	}

	if client != nil {
		client.disconnect()
		client.room.broadcastAllClientState()

		if err := scanner.Err(); err != nil {
			log.Printf("Client %v disconnected with error: %v", client.id, err)
		} else {
			log.Printf("Client %v disconnected\n", client.id)
		}
	} else {
		log.Println("Unknown client disconnected.")
	}

}

func (s *Server) findOrCreateClient(packet string, conn net.Conn) *Client {
	clientId := gjson.Get(packet, "clientId").Uint()

	s.mu.Lock()
	// if client id is 0, the client is new and we need to assign a new id
	if clientId == 0 {
		clientId = s.nextClientId
		s.nextClientId++
		//ensure clientId is never set to 0 if an overflow happens
		if s.nextClientId == 0 {
			s.nextClientId++
		}
	}

	// Check if the client id is already in use and look for a new one
	for {
		if _, ok := s.onlineClients[clientId]; !ok {
			break
		}
		clientId = s.nextClientId
		s.nextClientId++
		//ensure clientId is never set to 0 if an overflow happens
		if s.nextClientId == 0 {
			s.nextClientId++
		}
	}
	s.mu.Unlock()

	room := s.findOrCreateRoom(packet, clientId)
	team := room.findOrCreateTeam(gjson.Get(packet, "clientState.teamId").String())

	room.mu.Lock()

	client, ok := room.clients[clientId]
	clientState, _ := sjson.Set(gjson.Get(packet, "clientState").Raw, "clientId", clientId)
	if ok {
		client.mu.Lock()
		client.conn = conn
		client.state = clientState
		client.team = team
		client.lastActivity = time.Now()
		client.mu.Unlock()
	} else {
		client = &Client{
			id:           clientId,
			conn:         conn,
			server:       s,
			room:         room,
			team:         team,
			state:        clientState,
			lastActivity: time.Now(),
		}
		room.clients[clientId] = client
	}
	room.mu.Unlock()

	s.mu.Lock()
	s.onlineClients[clientId] = client
	s.mu.Unlock()

	return client
}

func (s *Server) findOrCreateRoom(packet string, clientId uint64) *Room {
	roomId := gjson.Get(packet, "roomId").String()

	s.mu.Lock()
	room, ok := s.rooms[roomId]
	if !ok {
		room = NewRoom(roomId, clientId, packet)
		s.rooms[roomId] = room
	}
	s.mu.Unlock()

	return room
}

func splitNullByte(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (s *Server) getSHA(conn net.Conn) string {
	//SHA256
	hash := sha256.Sum256([]byte(strings.Split(conn.RemoteAddr().String(), ":")[0]))
	return hex.EncodeToString(hash[:])
}

func (s *Server) banIP(ip string) {
	log.Println("Banning IP:" + ip)
	s.mu.Lock()
	s.banList = append(s.banList, ip)
	s.mu.Unlock()
}

func (s *Server) unbanIP(ip string) {
	log.Println("unbanning IP:" + ip)
	s.mu.Lock()
	tempBanList := make([]string, 0)
	for _, value := range s.banList {
		if value == ip {
			continue
		}
		tempBanList = append(tempBanList, value)
	}
	s.banList = tempBanList
	s.mu.Unlock()
}

func (s *Server) handleBannedConnection(conn net.Conn) {
	if conn != nil {
		conn.Write(append([]byte(`{"type":"SERVER_MESSAGE","message":"You have been banned from this Anchor server. If you believe this has been wrongfully done contact whoever is hosting it."}`), 0))
		conn.Write(append([]byte(`{"type":"DISABLE_ANCHOR"}`), 0))
		conn.Close()
	}
}
