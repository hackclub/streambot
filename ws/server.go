package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Activity struct {
	Type        string    `json:"type"`
	ChannelName string    `json:"channel,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

func NewActivity(activityType, channel string) Activity {
	return Activity{
		Type:        activityType,
		ChannelName: channel,
		Timestamp:   time.Now(),
	}
}

type Server struct {
	hub  *Hub
	port string
}

func NewServer(port string) *Server {
	s := Server{}

	s.hub = newHub()
	s.port = port

	return &s
}

func (s *Server) Serve() {
	go s.hub.run()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveWs(s.hub, w, r)
	})

	fmt.Println("websocket server listening on :" + s.port)
	err := http.ListenAndServe(":"+s.port, nil)
	if err != nil {
		log.Fatal("error hosting websocket server: ", err)
	}
}

func (s *Server) Broadcast(obj interface{}) {
	toSend, err := json.Marshal(obj)
	if err != nil {
		fmt.Println("error encoding json for ws:", err)
		return
	}

	fmt.Println("broadcasting msg type to ws...")
	s.hub.broadcast <- toSend
}
