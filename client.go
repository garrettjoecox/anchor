package main

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type Client struct {
	id           uint64
	conn         net.Conn
	server       *Server
	room         *Room
	team         *Team
	state        string     // Client state, current scene, etc.
	mu           sync.Mutex // Mutex for safely updating state
	lastActivity time.Time
}

func (c *Client) handlePacket(packet string) {
	c.lastActivity = time.Now()

	packetType := gjson.Get(packet, "type").String()

	if !c.server.quietMode.Load() && !gjson.Get(packet, "quiet").Exists() {
		log.Printf("Client %d -> Server: %s\n", c.id, packetType)
	}

	if packetType == "UPDATE_CLIENT_STATE" {
		c.mu.Lock()
		c.state = gjson.Get(packet, "state").Raw
		c.state, _ = sjson.Set(c.state, "clientId", c.id)
		c.mu.Unlock()

		team := c.room.findOrCreateTeam(gjson.Get(packet, "state.teamId").String())

		c.team = team
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
	if conn == nil {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	// Set write deadline to prevent blocking on dead connections
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := conn.Write(append([]byte(packet), 0))
	conn.SetWriteDeadline(time.Time{}) // Clear deadline

	if err != nil {
		c.disconnect()
	} else {
		c.mu.Lock()
		c.lastActivity = time.Now()
		c.mu.Unlock()
	}
}

func (c *Client) disconnect() {
	if c.conn != nil {
		c.conn.Close()
	}

	c.mu.Lock()
	c.state, _ = sjson.Set(c.state, "online", false)
	c.state, _ = sjson.Set(c.state, "isSaveLoaded", false)
	c.conn = nil
	c.mu.Unlock()

	c.server.onlineClients.Delete(c.id)
}

func (c *Client) sendRoomState() {
	if c.conn == nil {
		return
	}

	c.room.mu.Lock()
	packet, _ := sjson.SetRaw(`{"type":"UPDATE_ROOM_STATE"}`, "state", c.room.state)
	c.room.mu.Unlock()

	c.sendPacket(packet)
}
