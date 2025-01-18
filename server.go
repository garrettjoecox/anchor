package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const JSON_TEMPLATE = `{"gameCompleteCount":0,"onlineCount":0,"lastStatsHeartbeat":"","uniqueCount":0}`
const INACTIVITY_TIMEOUT = 5 * time.Minute
const HEARTBEAT = 30 * time.Second

type Server struct {
	listener          net.Listener
	quietMode         atomic.Bool
	onlineClients     sync.Map
	rooms             sync.Map
	gameCompleteCount atomic.Uint64
	nextClientId      atomic.Uint64
}

func NewServer() *Server {
	return &Server{
		onlineClients:     sync.Map{},
		quietMode:         atomic.Bool{},
		rooms:             sync.Map{},
		gameCompleteCount: atomic.Uint64{},
		nextClientId:      atomic.Uint64{},
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
	go s.parseStats(errChan)
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
	s.gameCompleteCount.Store(gjson.Get(string(value), "gamesComplete").Uint())
	s.nextClientId.Store(gjson.Get(string(value), "uniqueCount").Uint())
}

func (s *Server) onlineCount() int {
	var count int
	s.onlineClients.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

func (s *Server) saveStats() {
	value, _ := sjson.Set(JSON_TEMPLATE, "gamesComplete", s.gameCompleteCount.Load())
	value, _ = sjson.Set(value, "uniqueCount", s.nextClientId.Load())
	value, _ = sjson.Set(value, "onlineCount", s.onlineCount())
	value, _ = sjson.Set(value, "lastStatsHeartbeat", time.Now())

	err := os.WriteFile("./stats.json", []byte(value), 0644)

	if err != nil {
		log.Println("Error writing json to file: ", err)
	}
}

func (s *Server) cleanupInactiveRooms(errChan chan error) {
	ticker := time.NewTicker(HEARTBEAT)
	defer ticker.Stop()
	defer func() {
		if r := recover(); r != nil {
			errChan <- fmt.Errorf("panic in cleanupInactiveRooms: %v", r)
		}
	}()

	for range ticker.C {
		s.rooms.Range(func(id, value interface{}) bool {
			room := value.(*Room)
			lastActivity := room.GetLastActivity()
			if time.Since(lastActivity) > INACTIVITY_TIMEOUT {
				log.Println("Room", id, "has been inactive for too long, deleting it")
				s.rooms.Delete(id)
			}
			return true
		})
	}
}

func (s *Server) statsHeartbeat(errChan chan error) {
	ticker := time.NewTicker(HEARTBEAT)
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
	ticker := time.NewTicker(HEARTBEAT)
	defer ticker.Stop()
	defer func() {
		if r := recover(); r != nil {
			errChan <- fmt.Errorf("panic in heartbeat: %v", r)
		}
	}()

	for range ticker.C {
		if !s.quietMode.Load() {
			log.Println("Clients Online & Threads Running", s.onlineCount(), runtime.NumGoroutine())
		}

		s.onlineClients.Range(func(_, value interface{}) bool {
			client := value.(*Client)
			if time.Since(client.lastActivity) > HEARTBEAT {
				go client.sendPacket(`{"type":"HEARTBEAT","quiet":true}`)
			}
			return true
		})
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

	var client *Client

	for scanner.Scan() {
		packet := scanner.Text()

		if !gjson.Valid(packet) {
			log.Printf("Invalid JSON packet: %s\n", packet)
			continue
		}

		packetTypeWrapped := gjson.Get(packet, "type")
		if !packetTypeWrapped.Exists() {
			log.Println("Packet missing type")
			continue
		}

		packetType := packetTypeWrapped.String()

		// Health check
		if packetType == "STATS" {
			outgoingPacket, _ := sjson.Set(`{"type":"STATS"}`, "uniqueCount", s.nextClientId.Load())
			outgoingPacket, _ = sjson.Set(outgoingPacket, "gameCompleteCount", s.gameCompleteCount.Load())
			outgoingPacket, _ = sjson.Set(outgoingPacket, "onlineCount", s.onlineCount())
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

	// Check if the client id is already in use or is 0 and look for a new one
	for {
		if _, ok := s.onlineClients.Load(clientId); !ok && clientId != 0 {
			break
		}
		clientId = s.nextClientId.Add(1)
	}

	room := s.findOrCreateRoom(packet, clientId)
	team := room.findOrCreateTeam(gjson.Get(packet, "clientState.teamId").String())

	var client *Client
	loadedClient, ok := room.clients.Load(clientId)
	clientState, _ := sjson.Set(gjson.Get(packet, "clientState").Raw, "clientId", clientId)
	if ok {
		client = loadedClient.(*Client)
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
		room.clients.Store(clientId, client)
	}

	s.onlineClients.Store(clientId, client)

	return client
}

func (s *Server) findOrCreateRoom(packet string, clientId uint64) *Room {
	roomId := gjson.Get(packet, "roomId").String()

	room, ok := s.rooms.Load(roomId)
	if !ok {
		room = NewRoom(roomId, clientId, packet)
		s.rooms.Store(roomId, room)
	}

	return room.(*Room)
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
