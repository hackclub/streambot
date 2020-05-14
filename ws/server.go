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
	}
}

type Server struct {
	hub  *Hub
	port string
}

func NewServer(port string) Server {
	s := Server{}

	s.hub = newHub()
	s.port = port

	return s
}

func (s *Server) Serve() {
	hub := newHub()
	go hub.run()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	go func() {
		fmt.Println("websocket server listening on :" + s.port)
		err := http.ListenAndServe(":"+s.port, nil)
		if err != nil {
			log.Fatal("error with websocket server: ", err)
		}
	}()
}

func (s *Server) Broadcast(obj interface{}) {
	toSend, err := json.Marshal(obj)
	if err != nil {
		fmt.Println("error encoding json for ws:", err)
	} else {
		fmt.Println("broadcasting msg type to ws...")
		s.hub.broadcast <- toSend
	}
}
