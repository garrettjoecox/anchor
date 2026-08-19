package main

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const sendQueueSize = 256

type Client struct {
	id           uint64
	conn         net.Conn
	sendCh       chan string // Outgoing packet queue, drained by the connection's writeLoop
	server       *Server
	room         *Room
	team         *Team
	state        string     // Client state, current scene, etc.
	mu           sync.Mutex // Mutex for safely updating state
	lastActivity time.Time
}

func (c *Client) attachConnLocked(conn net.Conn) {
	c.conn = conn
	c.sendCh = make(chan string, sendQueueSize)
	go c.writeLoop(conn, c.sendCh)
}

func (c *Client) writeLoop(conn net.Conn, ch chan string) {
	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic in writeLoop for client %d: %v", c.id, r)
		}
	}()

	for packet := range ch {
		// Set write deadline to prevent blocking on dead connections
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err := conn.Write(append([]byte(packet), 0))
		conn.SetWriteDeadline(time.Time{}) // Clear deadline

		if err != nil {
			c.disconnectConn(conn)
			return
		}

		c.mu.Lock()
		c.lastActivity = time.Now()
		c.mu.Unlock()
	}
}

func (c *Client) handlePacket(packet string) {
	c.mu.Lock()
	c.lastActivity = time.Now()
	c.mu.Unlock()

	packetType := gjson.Get(packet, "type").String()

	if !c.server.quietMode.Load() && !gjson.Get(packet, "quiet").Exists() {
		log.Printf("Client %d -> Server: %s\n", c.id, packetType)
	}

	if packetType == "UPDATE_CLIENT_STATE" {
		team := c.room.findOrCreateTeam(gjson.Get(packet, "state.teamId").String())

		c.mu.Lock()
		c.state = gjson.Get(packet, "state").Raw
		c.state, _ = sjson.Set(c.state, "clientId", c.id)
		c.team = team
		c.mu.Unlock()
	}

	if packetType == "GAME_COMPLETE" {
		c.server.gameCompleteCount.Add(1)
	}

	targetClientId := gjson.Get(packet, "targetClientId")

	if targetClientId.Exists() {
		value, ok := c.room.clients.Load(targetClientId.Uint())
		if ok {
			targetClient := value.(*Client)
			targetClient.sendPacket(packet)
		}
		return
	}

	targetTeamId := gjson.Get(packet, "targetTeamId")

	if packetType == "REQUEST_TEAM_STATE" {
		if !targetTeamId.Exists() {
			return
		}

		team := c.room.findOrCreateTeam(targetTeamId.String())
		teamMemberOnline := false
		c.room.clients.Range(func(_, value interface{}) bool {
			client := value.(*Client)
			client.mu.Lock()
			if client.id != c.id && client.conn != nil && client.team == team && gjson.Get(client.state, "isSaveLoaded").Bool() {
				teamMemberOnline = true
			}
			client.mu.Unlock()
			return true
		})

		if teamMemberOnline {
			team.mu.Lock()
			team.clientIdsRequestingState = append(team.clientIdsRequestingState, c.id)
			team.mu.Unlock()
			team.broadcastPacket(packet)
			return
		}

		// Teammate is offline, see if we have a saved state for the team
		outgoingPacket := `{"type": "UPDATE_TEAM_STATE"}`
		team.mu.Lock()
		if team.state != "{}" {
			outgoingPacket, _ = sjson.SetRaw(outgoingPacket, "state", team.state)
		}
		outgoingPacket, _ = sjson.Set(outgoingPacket, "queue", team.queue)
		team.mu.Unlock()

		c.sendPacket(outgoingPacket)
	} else if packetType == "UPDATE_TEAM_STATE" {
		if !targetTeamId.Exists() {
			return
		}

		team := c.room.findOrCreateTeam(targetTeamId.String())

		team.mu.Lock()
		clientIdsRequestingState := team.clientIdsRequestingState
		team.state = gjson.Get(packet, "state").Raw
		team.queue = []string{}
		team.clientIdsRequestingState = []uint64{}
		team.mu.Unlock()

		for _, clientId := range clientIdsRequestingState {
			if value, ok := c.room.clients.Load(clientId); ok {
				client := value.(*Client)
				client.sendPacket(packet)
			}
		}

	} else if packetType == "UPDATE_ROOM_STATE" {
		c.room.mu.Lock()
		c.room.state = gjson.Get(packet, "state").Raw
		c.room.mu.Unlock()
		c.room.broadcastPacket(packet)
	} else if targetTeamId.Exists() {
		team := c.room.findOrCreateTeam(targetTeamId.String())
		addToQueue := gjson.Get(packet, "addToQueue")

		if addToQueue.Exists() && addToQueue.Bool() {
			team.mu.Lock()
			team.queue = append(team.queue, packet)
			team.mu.Unlock()
		}

		team.broadcastPacket(packet)
	} else {
		c.room.broadcastPacket(packet)
	}
}

func (c *Client) sendPacket(packet string) {
	if !c.server.quietMode.Load() && !gjson.Get(packet, "quiet").Exists() {
		log.Printf("Client %d <- Server: %s\n", c.id, gjson.Get(packet, "type").String())
	}

	// Lock to prevent race condition with disconnect
	c.mu.Lock()
	conn := c.conn
	ch := c.sendCh
	c.mu.Unlock()
	if conn == nil || ch == nil {
		return
	}

	// Enqueue for the connection's writer goroutine; never blocks
	select {
	case ch <- packet:
	default:
		// Queue full, the client isn't draining its socket, consider the session dead
		log.Printf("Client %d send queue full, disconnecting\n", c.id)
		c.disconnectConn(conn)
	}
}

func (c *Client) disconnect() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	c.disconnectConn(conn)
}

func (c *Client) disconnectConn(conn net.Conn) {
	if conn == nil {
		return
	}

	c.mu.Lock()
	if c.conn != conn {
		c.mu.Unlock()
		return
	}
	c.state, _ = sjson.Set(c.state, "online", false)
	c.state, _ = sjson.Set(c.state, "isSaveLoaded", false)
	c.conn = nil
	if c.sendCh != nil {
		close(c.sendCh)
		c.sendCh = nil
	}
	c.mu.Unlock()

	c.server.onlineClients.Delete(c.id)
}

func (c *Client) sendRoomState() {
	c.room.mu.Lock()
	packet, _ := sjson.SetRaw(`{"type":"UPDATE_ROOM_STATE"}`, "state", c.room.state)
	c.room.mu.Unlock()

	c.sendPacket(packet)
}
