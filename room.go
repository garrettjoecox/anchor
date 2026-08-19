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
	value, ok := r.teams.Load(teamId)
	if !ok {
		value, _ = r.teams.LoadOrStore(teamId, &Team{
			id:    teamId,
			state: "{}",
			room:  r,
			queue: make([]string, 0),
		})
	}

	return value.(*Team)
}

func (r *Room) broadcastPacket(packet string) {
	clientId := gjson.Get(packet, "clientId").Uint()

	r.clients.Range(func(_, value interface{}) bool {
		client := value.(*Client)
		if client.id != clientId {
			client.sendPacket(packet)
		}

		return true
	})
}

func (r *Room) broadcastAllClientState() {
	r.mu.Lock()
	defer r.mu.Unlock()

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
		clientPacket, _ := sjson.Set(packet, "state."+fmt.Sprint(idToIndex[id])+".self", true)
		client.sendPacket(clientPacket)
		return true
	})
}

func (r *Room) GetLastActivity() time.Time {
	var lastActivity time.Time

	r.clients.Range(func(id, value interface{}) bool {
		client := value.(*Client)
		client.mu.Lock()
		if client.lastActivity.After(lastActivity) {
			lastActivity = client.lastActivity
		}
		client.mu.Unlock()
		return true
	})

	return lastActivity
}
