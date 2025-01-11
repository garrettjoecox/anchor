package main

import (
	"fmt"
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

	if !gjson.Get(packet, "quiet").Exists() {
		fmt.Printf("Client %d -> Server: %s\n", c.id, packetType)
	}

	if packetType == "UPDATE_CLIENT_STATE" {
		c.mu.Lock()
		c.state = gjson.Get(packet, "state").Raw
		c.state, _ = sjson.Set(c.state, "clientId", c.id)
		teamId := gjson.Get(packet, "state.teamId").String()
		c.team = c.room.findOrCreateTeam(teamId)
		c.mu.Unlock()
	}

	if packetType == "GAME_COMPLETE" {
		c.server.mu.Lock()
		c.server.gamesCompleted++
		c.server.mu.Unlock()
	}

	targetClientId := gjson.Get(packet, "targetClientId")

	if targetClientId.Exists() {
		if targetClient, ok := c.room.clients[targetClientId.Uint()]; ok {
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
		c.room.mu.Lock()
		for _, client := range c.room.clients {
			if client.id != c.id && client.conn != nil && client.team == team && gjson.Get(client.state, "isSaveLoaded").Bool() {
				teamMemberOnline = true
			}
		}
		c.room.mu.Unlock()

		if teamMemberOnline {
			team.mu.Lock()
			team.clientIdsRequestingState = append(team.clientIdsRequestingState, c.id)
			team.mu.Unlock()
			team.broadcastPacket(packet)
			return
		}

		// Teammate is offline, see if we have a saved state for the team
		team.mu.Lock()
		outgoingPacket := `{"type": "UPDATE_TEAM_STATE"}`
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
		team.state = gjson.Get(packet, "state").Raw
		team.queue = []string{}

		for _, clientId := range team.clientIdsRequestingState {
			if client, ok := c.room.clients[clientId]; ok {
				client.sendPacket(packet)
			}
		}

		team.clientIdsRequestingState = []uint64{}
		team.mu.Unlock()
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
	if c.conn == nil {
		return
	}

	if !gjson.Get(packet, "quiet").Exists() {
		fmt.Printf("Client %d <- Server: %s\n", c.id, gjson.Get(packet, "type").String())
	}

	_, err := c.conn.Write(append([]byte(packet), 0))

	if err != nil {
		c.disconnect()
	} else {
		c.lastActivity = time.Now()
	}
}

func (c *Client) disconnect() {
	if c.conn != nil {
		c.conn.Close()
	}

	c.mu.Lock()
	c.state, _ = sjson.Set(c.state, "online", false)
	c.conn = nil
	c.mu.Unlock()

	c.server.mu.Lock()
	delete(c.server.onlineClients, c.id)
	c.server.mu.Unlock()
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
