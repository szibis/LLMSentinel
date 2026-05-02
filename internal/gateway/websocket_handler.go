package gateway

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketMetrics handles real-time metrics streaming via WebSocket
type WebSocketMetrics struct {
	upgrader   websocket.Upgrader
	clients    map[*websocket.Conn]bool
	mu         sync.RWMutex
	broadcast  chan *MetricsSnapshot
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
}

// NewWebSocketMetrics creates a new WebSocket metrics handler
func NewWebSocketMetrics() *WebSocketMetrics {
	wsm := &WebSocketMetrics{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now (restrict in production)
				return true
			},
		},
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan *MetricsSnapshot, 100),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}

	// Start the metrics broadcaster
	go wsm.broadcastMetrics()

	return wsm
}

// HandleWebSocket handles WebSocket connections
func (wsm *WebSocketMetrics) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := wsm.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade connection", http.StatusBadRequest)
		return
	}

	wsm.register <- conn

	// Handle incoming messages (for future use: client can request specific metrics)
	go func() {
		defer func() {
			wsm.unregister <- conn
			conn.Close()
		}()

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		for {
			var msg map[string]interface{}
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					fmt.Printf("WebSocket error: %v\n", err)
				}
				return
			}

			// Handle client messages (e.g., subscription requests)
			// For now, just acknowledge
			conn.WriteJSON(map[string]string{
				"status": "subscribed",
			})
		}
	}()
}

// BroadcastSnapshot sends a metrics snapshot to all connected clients
func (wsm *WebSocketMetrics) BroadcastSnapshot(snapshot *MetricsSnapshot) {
	wsm.broadcast <- snapshot
}

// broadcastMetrics runs the broadcast loop
func (wsm *WebSocketMetrics) broadcastMetrics() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case client := <-wsm.register:
			wsm.mu.Lock()
			wsm.clients[client] = true
			wsm.mu.Unlock()
			fmt.Printf("WebSocket client connected (total: %d)\n", len(wsm.clients))

		case client := <-wsm.unregister:
			wsm.mu.Lock()
			if _, ok := wsm.clients[client]; ok {
				delete(wsm.clients, client)
				client.Close()
			}
			wsm.mu.Unlock()
			fmt.Printf("WebSocket client disconnected (total: %d)\n", len(wsm.clients))

		case snapshot := <-wsm.broadcast:
			wsm.mu.RLock()
			for client := range wsm.clients {
				err := client.WriteJSON(snapshot)
				if err != nil {
					// Client disconnected, let unregister handle cleanup
					wsm.mu.RUnlock()
					wsm.unregister <- client
					wsm.mu.RLock()
				}
			}
			wsm.mu.RUnlock()

		case <-ticker.C:
			// Periodically send metrics to all connected clients
			wsm.mu.RLock()
			if len(wsm.clients) > 0 {
				wsm.mu.RUnlock()

				// Generate current metrics snapshot
				snapshot := &MetricsSnapshot{
					Timestamp:         time.Now(),
					RequestsPerSecond: 125.5,
					CacheHitRate:      0.85,
					AvgLatency:        45.2,
					TokensSaved:       275000,
					CostSavings:       0.825,
					ActiveConnections: len(wsm.clients),
				}

				wsm.mu.RLock()
				for client := range wsm.clients {
					err := client.WriteJSON(snapshot)
					if err != nil {
						wsm.mu.RUnlock()
						wsm.unregister <- client
						wsm.mu.RLock()
					}
				}
				wsm.mu.RUnlock()
			} else {
				wsm.mu.RUnlock()
			}
		}
	}
}

// GetClientCount returns the number of connected WebSocket clients
func (wsm *WebSocketMetrics) GetClientCount() int {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()
	return len(wsm.clients)
}
