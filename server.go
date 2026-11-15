package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const INACTIVITY_TIMEOUT = 5 * time.Minute
const HEARTBEAT = 30 * time.Second
const MAX_PACKET_SIZE = 8 * 1024 * 1024
const INITIAL_SCAN_BUFFER = 64 * 1024

type Server struct {
	listener      net.Listener
	quietMode     atomic.Bool
	onlineClients sync.Map
	rooms         sync.Map
	nextClientId  atomic.Uint64
}

func NewServer() *Server {
	s := &Server{
		onlineClients: sync.Map{},
		quietMode:     atomic.Bool{},
		rooms:         sync.Map{},
		nextClientId:  atomic.Uint64{},
	}

	s.quietMode.Store(true)

	return s
}

func (s *Server) Start(errChan chan error) {
	listener, err := net.Listen("tcp", ":43383")
	if err != nil {
		log.Fatal(err)
	}
	s.listener = listener

	go s.cleanupInactiveRooms(errChan)
	go s.heartbeat(errChan)

	log.Println("Server running on :43383")
	log.Println("Quiet mode:", s.quietMode.Load())

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Println("Error with listener:", err)
				break
			}
			log.Println("Error accepting connection:", err)
			time.Sleep(time.Second)
			continue
		}

		go s.handleConnection(conn, errChan)
	}
}

func (s *Server) onlineCount() int {
	var count int
	s.onlineClients.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
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

func (s *Server) heartbeat(errChan chan error) {
	ticker := time.NewTicker(HEARTBEAT)
	defer ticker.Stop()
	defer func() {
		if r := recover(); r != nil {
			errChan <- fmt.Errorf("panic in heartbeat: %v", r)
		}
	}()

	for range ticker.C {
		log.Println("Clients Online & Threads Running", s.onlineCount(), runtime.NumGoroutine())

		s.onlineClients.Range(func(_, value interface{}) bool {
			client := value.(*Client)
			client.mu.Lock()
			idle := time.Since(client.lastActivity) > HEARTBEAT
			client.mu.Unlock()
			if idle {
				client.sendPacket(`{"type":"HEARTBEAT","quiet":true}`)
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
	scanner.Buffer(make([]byte, 0, INITIAL_SCAN_BUFFER), MAX_PACKET_SIZE)
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
		client.disconnectConn(conn)
		client.room.broadcastAllClientState()

		if err := scanner.Err(); err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				log.Printf("Client %v sent a packet over the %d byte limit, disconnecting", client.id, MAX_PACKET_SIZE)
			} else {
				log.Printf("Client %v disconnected with error: %v", client.id, err)
			}
		} else {
			log.Printf("Client %v disconnected\n", client.id)
		}
	} else if !s.quietMode.Load() {
		log.Println("Unknown client disconnected.")
	}

}

func (s *Server) findOrCreateClient(packet string, conn net.Conn) *Client {
	clientId := gjson.Get(packet, "clientId").Uint()
	roomId := gjson.Get(packet, "roomId").String()

	takeover := false
	if clientId != 0 {
		if value, ok := s.onlineClients.Load(clientId); ok {
			existing := value.(*Client)
			if existing.room.id == roomId {
				takeover = true
				log.Printf("Client %v reconnected, closing stale session\n", clientId)
				existing.disconnect()
			} else {
				clientId = 0
			}
		}
	}

	// Check if the client id is already in use or is 0 and look for a new one
	for !takeover {
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
		client.attachConnLocked(conn)
		client.state = clientState
		client.team = team
		client.lastActivity = time.Now()
		client.mu.Unlock()
	} else {
		client = &Client{
			id:           clientId,
			server:       s,
			room:         room,
			team:         team,
			state:        clientState,
			lastActivity: time.Now(),
		}
		client.mu.Lock()
		client.attachConnLocked(conn)
		client.mu.Unlock()
		room.clients.Store(clientId, client)
	}

	s.onlineClients.Store(clientId, client)

	return client
}

func (s *Server) findOrCreateRoom(packet string, clientId uint64) *Room {
	roomId := gjson.Get(packet, "roomId").String()

	room, ok := s.rooms.Load(roomId)
	if !ok {
		room, _ = s.rooms.LoadOrStore(roomId, NewRoom(roomId, clientId, packet))
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
