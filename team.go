package main

import (
	"sync"

	"github.com/tidwall/gjson"
)

type Team struct {
	id                       string
	clientIdsRequestingState []uint64
	room                     *Room
	state                    string     // Save state
	queue                    []string   // Packet queue to apply to Save
	mu                       sync.Mutex // Mutex for safely updating state/queue
}

func (t *Team) broadcastPacket(packet string) {
	t.room.mu.Lock()
	defer t.room.mu.Unlock()

	clientId := gjson.Get(packet, "clientId").Uint()

	for _, client := range t.room.clients {
		if client.conn == nil || client.id == clientId || client.team != t {
			continue
		}

		client.sendPacket(packet)
	}
}
