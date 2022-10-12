package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/", handleWebSocket)
	router.Use(mux.CORSMethodMiddleware(router))
	log.Fatalln(http.ListenAndServe(":5005", router))
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Event struct {
	Command string      `json:"command"`
	Data    interface{} `json:"data"`
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	var conn *websocket.Conn

	if upgradedConn, err := upgrader.Upgrade(w, r, nil); err != nil {
		log.Printf("Could not upgrade connection: %v", err.Error())
	} else {
		conn = upgradedConn
	}

	defer func() {
		_ = conn.Close()

		log.Print("Socket disconnected")
	}()

	log.Print("Socket connected")

	for {
		var event Event
		if err := conn.ReadJSON(&event); err != nil {
			log.Printf("Could not read json: %v", err.Error())

			break
		}

		switch event.Command {
		case "face-capture-frame":
			data := fmt.Sprintf("%v", event.Data)
			log.Print("Serving frame")
			serveFrame(data)
		}
	}
}

func serveFrame(rawImage string) {
	decodedString, decodeErr := base64.StdEncoding.DecodeString(rawImage)

	if decodeErr != nil {
		log.Printf("Could not decode base64 string: %v", decodeErr.Error())

		return
	}

	reader := bytes.NewReader(decodedString)

	img, err := jpeg.Decode(reader)

	if err != nil {
		log.Printf("Could not decode image: %v", err.Error())

		return
	}

	out, _ := os.Create(fmt.Sprintf("./%v.jpeg", time.Now().UnixMilli()))
	defer func() {
		_ = out.Close()
	}()

	if err = jpeg.Encode(out, img, &jpeg.Options{Quality: 10}); err != nil {
		log.Printf("Could not encode image: %v", err.Error())
	}

}
