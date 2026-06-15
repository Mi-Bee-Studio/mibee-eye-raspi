package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/camera"
	"github.com/gorilla/websocket"
)

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = 30 * time.Second
	wsMaxMsgSize = 512
)

func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Non-browser clients (curl, scripts, tools)
	}
	// Allow if origin matches the request Host
	return origin == "http://"+r.Host || origin == "https://"+r.Host
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

// wsHub maintains a set of active WebSocket connections and broadcasts events.
type wsHub struct {
	logger  *log.Logger
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
	events  chan wsEvent
	done    chan struct{}
	closeOnce sync.Once
}

// wsClient wraps a WebSocket connection with a per-connection write mutex.
// gorilla/websocket only supports one concurrent writer; this mutex prevents
// races between hub broadcast, ping writes, and initial state sends.
type wsClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsClient) writeMT(msgType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
	return c.conn.WriteMessage(msgType, data)
}


func newWSHub(logger *log.Logger) *wsHub {
	return &wsHub{
		logger:  logger,
		clients: make(map[*wsClient]struct{}),
		events:  make(chan wsEvent, 64),
		done:    make(chan struct{}),
	}
}

// sendEvent queues an event for broadcast. Non-blocking — drops if full.
func (h *wsHub) sendEvent(e wsEvent) {
	select {
	case h.events <- e:
	default:
		// Drop event if channel is full — non-blocking for hook callbacks.
	}
}

// run consumes events from the channel and broadcasts to all clients.
func (h *wsHub) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.done:
			return
		case e := <-h.events:
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}

			// Collect failed connections under read lock, then remove under write lock.
			h.mu.RLock()
			var failed []*wsClient
			for client := range h.clients {
				if err := client.writeMT(websocket.TextMessage, data); err != nil {
					failed = append(failed, client)
				}
			}
			h.mu.RUnlock()

			if len(failed) > 0 {
				h.mu.Lock()
			for _, client := range failed {
				if _, still := h.clients[client]; still {
					client.conn.Close()
					delete(h.clients, client)
				}
			}
				h.mu.Unlock()
			}
		}
	}
}

// close shuts down the hub and closes all connections.
func (h *wsHub) close() {
	h.closeOnce.Do(func() {
		close(h.done)
	})
	h.mu.Lock()
	for client := range h.clients {
		client.conn.Close()
	}
	h.clients = make(map[*wsClient]struct{})
	h.mu.Unlock()
}

// addClient registers a new WebSocket connection.
func (h *wsHub) addClient(client *wsClient) {
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
}

func (h *wsHub) removeClient(client *wsClient) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()
	client.conn.Close()
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Printf("web: WebSocket upgrade failed: %v", err)
		return
	}

	client := &wsClient{conn: conn}
	s.hub.addClient(client)
	go s.wsWritePump(client)
	go s.wsReadPump(client)
}

func (s *Server) wsReadPump(client *wsClient) {
	defer s.hub.removeClient(client)

	client.conn.SetReadLimit(wsMaxMsgSize)
	client.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	client.conn.SetPongHandler(func(string) error {
		client.conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})

	for {
		_, _, err := client.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (s *Server) wsWritePump(client *wsClient) {
	ticker := time.NewTicker(wsPingPeriod)
	defer func() {
		ticker.Stop()
		client.conn.Close()
	}()

	// Send initial state snapshot on connect.
	s.sendInitialState(client)

	for {
		select {
		case <-ticker.C:
			if err := client.writeMT(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
				return
			}
		}
	}
}

func (s *Server) sendInitialState(client *wsClient) {
	// Send current params.
	if s.cfg.Params != nil {
		for name := range camera.ParamRanges {
			if val, err := s.cfg.Params.Get(name); err == nil {
				msg, _ := json.Marshal(wsEvent{
					Type:  "param-changed",
					Name:  name,
					Value: val,
				})
				client.writeMT(websocket.TextMessage, msg)
			}
		}
		for name := range camera.ParamEnums {
			if val, err := s.cfg.Params.Get(name); err == nil {
				msg, _ := json.Marshal(wsEvent{
					Type:  "param-changed",
					Name:  name,
					Value: val,
				})
				client.writeMT(websocket.TextMessage, msg)
			}
		}
	}

	// Send PTZ position.
	if s.cfg.PTZ != nil {
		pos := s.cfg.PTZ.GetPosition()
		msg, _ := json.Marshal(map[string]interface{}{
			"type":     "ptz-position",
			"position": pos,
		})
		client.writeMT(websocket.TextMessage, msg)
	}

	// Send preset list.
	if s.cfg.PTZ != nil {
		tokens := s.cfg.PTZ.GetPresets()
		for _, token := range tokens {
			pos, err := s.cfg.PTZ.GetPresetPosition(token)
			if err != nil {
				continue
			}
			msg, _ := json.Marshal(map[string]interface{}{
				"type":     "ptz-preset-added",
				"token":    token,
				"name":     token,
				"position": pos,
			})
			client.writeMT(websocket.TextMessage, msg)
		}
	}
}
