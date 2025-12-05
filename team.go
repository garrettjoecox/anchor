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
	clientId := gjson.Get(packet, "clientId").Uint()

	t.room.clients.Range(func(_, value interface{}) bool {
		client := value.(*Client)
		if client.team == t && client.conn != nil && client.id != clientId {
			go client.sendPacket(packet)
		}

		return true
	})
}
