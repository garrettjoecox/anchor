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

	// sendPacket only enqueues onto the recipient's write queue, so this never blocks
	// on a slow client and packets keep their order
	t.room.clients.Range(func(_, value interface{}) bool {
		client := value.(*Client)
		client.mu.Lock()
		onTeam := client.team == t
		client.mu.Unlock()
		if onTeam && client.id != clientId {
			client.sendPacket(packet)
		}

		return true
	})
}
