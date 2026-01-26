package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type Room struct {
	id      string
	clients sync.Map
	teams   sync.Map
	state   string     // Room Settings
	mu      sync.Mutex // Mutex for safely updating state
}

func NewRoom(id string, ownerClientId uint64, packet string) *Room {
	roomState, _ := sjson.Set(gjson.Get(packet, "roomState").Raw, "ownerClientId", ownerClientId)

	return &Room{
		id:      id,
		clients: sync.Map{},
		teams:   sync.Map{},
		state:   roomState,
	}
}

func (r *Room) findOrCreateTeam(teamId string) *Team {
	var team *Team
	value, ok := r.teams.Load(teamId)
	if ok {
		team = value.(*Team)
	} else {
		team = &Team{
			id:    teamId,
			state: "{}",
			room:  r,
			queue: make([]string, 0),
		}
		r.teams.Store(teamId, team)
	}

	return team
}

func (r *Room) broadcastPacket(packet string) {
	clientId := gjson.Get(packet, "clientId").Uint()

	r.clients.Range(func(_, value interface{}) bool {
		client := value.(*Client)
		if client.conn != nil && client.id != clientId {
			go func(c *Client) {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Panic in sendPacket for client %d: %v", c.id, r)
					}
				}()
				c.sendPacket(packet)
			}(client)
		}

		return true
	})
}

func (r *Room) broadcastAllClientState() {
	packet := `{"type":"ALL_CLIENT_STATE","state":[]}`

	idToIndex := make(map[interface{}]int)
	index := 0

	r.clients.Range(func(id, value interface{}) bool {
		client := value.(*Client)
		idToIndex[id] = index
		client.mu.Lock()
		packet, _ = sjson.SetRaw(packet, "state."+fmt.Sprint(index), client.state)
		client.mu.Unlock()
		index++
		return true
	})

	r.clients.Range(func(id, value interface{}) bool {
		client := value.(*Client)
		if client.conn == nil {
			return true
		}

		clientPacket, _ := sjson.Set(packet, "state."+fmt.Sprint(idToIndex[id])+".self", true)

		go func(c *Client, p string) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Panic in sendPacket for client %d: %v", c.id, r)
				}
			}()
			c.sendPacket(p)
		}(client, clientPacket)

		return true
	})
}

func (r *Room) GetLastActivity() time.Time {
	var lastActivity time.Time

	r.clients.Range(func(id, value interface{}) bool {
		client := value.(*Client)
		if client.lastActivity.After(lastActivity) {
			lastActivity = client.lastActivity
		}
		return true
	})

	return lastActivity
}
