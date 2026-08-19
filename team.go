package main

import (
	"log"
	"sync"

	"github.com/tidwall/gjson"
)

const MAX_TEAM_QUEUE = 512

type Team struct {
	id                       string
	clientIdsRequestingState []uint64
	room                     *Room
	state                    string     // Save state
	queue                    []string   // Packet queue to apply to Save
	droppedFromQueue         int        // Oldest queued packets discarded since the last full state
	mu                       sync.Mutex // Mutex for safely updating state/queue
}

func (t *Team) enqueue(packet string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.queue = append(t.queue, packet)
	if len(t.queue) <= MAX_TEAM_QUEUE {
		return
	}

	dropped := len(t.queue) - MAX_TEAM_QUEUE
	copy(t.queue, t.queue[dropped:])
	for i := MAX_TEAM_QUEUE; i < len(t.queue); i++ {
		t.queue[i] = ""
	}
	t.queue = t.queue[:MAX_TEAM_QUEUE]

	if t.droppedFromQueue == 0 {
		log.Printf("Team %s queue hit %d packets, dropping oldest entries", t.id, MAX_TEAM_QUEUE)
	}
	t.droppedFromQueue += dropped
}

func (t *Team) broadcastPacket(packet string) {
	clientId := gjson.Get(packet, "clientId").Uint()

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
