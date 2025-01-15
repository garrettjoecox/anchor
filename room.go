package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type Room struct {
	id      string
	clients map[uint64]*Client
	teams   map[string]*Team
	state   string     // Room Settings
	mu      sync.Mutex // Mutex for safely updating state
}

func NewRoom(id string, ownerClientId uint64, packet string) *Room {
	roomState, _ := sjson.Set(gjson.Get(packet, "roomState").Raw, "ownerClientId", ownerClientId)

	return &Room{
		id:      id,
		clients: make(map[uint64]*Client),
		teams:   make(map[string]*Team),
		state:   roomState,
	}
}

func (r *Room) findOrCreateTeam(teamId string) *Team {
	r.mu.Lock()
	defer r.mu.Unlock()

	team, ok := r.teams[teamId]
	if !ok {
		team = &Team{
			id:    teamId,
			state: "{}",
			room:  r,
			queue: make([]string, 0),
		}
		r.teams[teamId] = team
	}

	return team
}

func (r *Room) broadcastPacket(packet string) {
	clientId := gjson.Get(packet, "clientId").Uint()

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, client := range r.clients {
		if client.conn == nil || client.id == clientId {
			continue
		}

		client.sendPacket(packet)
	}
}

func (r *Room) broadcastAllClientState() {
	r.mu.Lock()
	defer r.mu.Unlock()

	packet := `{"type":"ALL_CLIENT_STATE","state":[]}`

	idToIndex := make(map[uint64]int)
	index := 0

	for id, client := range r.clients {
		idToIndex[id] = index
		client.mu.Lock()
		packet, _ = sjson.SetRaw(packet, "state."+fmt.Sprint(index), client.state)
		client.mu.Unlock()
		index++
	}

	for id, client := range r.clients {
		if client.conn == nil {
			continue
		}

		packet, _ = sjson.Set(packet, "state."+fmt.Sprint(idToIndex[id])+".self", true)

		client.sendPacket(packet)

		packet, _ = sjson.Delete(packet, "state."+fmt.Sprint(idToIndex[id])+".self")
	}
}

func (r *Room) GetLastActivity() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()

	var lastActivity time.Time

	for _, client := range r.clients {
		client.mu.Lock()
		if client.lastActivity.After(lastActivity) {
			lastActivity = client.lastActivity
		}
		client.mu.Unlock()
	}

	return lastActivity
}
