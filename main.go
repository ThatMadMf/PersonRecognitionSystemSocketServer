package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
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
	Uuid    uuid.UUID   `json:"uuid"`
	Command string      `json:"command"`
	Data    interface{} `json:"data"`
}

type EventResponse struct {
	Uuid    uuid.UUID   `json:"uuid"`
	Command string      `json:"command"`
	Result  string      `json:"result"`
	Data    interface{} `json:"data"`
}

type SocketContext struct {
	connType          string
	deviceId          string
	authorizationUUID uuid.UUID
}

type Socket struct {
	conn    *websocket.Conn
	context SocketContext
}

type DeviceAuthorizationDto struct {
	DeviceId  string `json:"deviceId"`
	AuthToken string `json:"authToken"`
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	var conn *websocket.Conn

	if upgradedConn, err := upgrader.Upgrade(w, r, nil); err != nil {
		log.Printf("Could not upgrade connection: %v", err.Error())
	} else {
		conn = upgradedConn
	}

	socket := Socket{conn: conn}

	defer func() {
		_ = conn.Close()

		log.Print("Socket disconnected")
	}()

	log.Print("Socket connected")

	for {
		var event Event
		if err := socket.conn.ReadJSON(&event); err != nil {
			log.Printf("Could not read json: %v", err.Error())

			break
		}

		var err error
		var response EventResponse

		switch event.Command {
		case "authorize-device":
			response, err = authorizeDevice(&socket, event)
		case "face-capture-frame":
			data := fmt.Sprintf("%v", event.Data)
			log.Print("Serving frame")
			serveFrame(data)
		}

		if err != nil {
			log.Printf(
				"Error during event %s: %v for socket %v",
				event.Command,
				err.Error(),
				socket.context.deviceId,
			)
			response = EventResponse{
				Uuid:    event.Uuid,
				Command: event.Command,
				Result:  "error",
				Data:    err.Error(),
			}
		}

		if err = socket.conn.WriteJSON(response); err != nil {
			log.Printf("Could not send response to socket %s: %v", socket.context.deviceId, socket)
		}

	}
}

func authorizeDevice(socket *Socket, event Event) (EventResponse, error) {
	var dto DeviceAuthorizationDto

	if err := mapstructure.Decode(event.Data, &dto); err != nil {
		return EventResponse{}, err
	}

	token, err := uuid.Parse(dto.AuthToken)

	if err != nil {
		return EventResponse{}, err
	}

	socket.context = SocketContext{
		connType:          "INPUT_DEVICE",
		deviceId:          dto.DeviceId,
		authorizationUUID: token,
	}

	return EventResponse{
		Uuid:    event.Uuid,
		Command: event.Command,
		Result:  "success",
	}, nil
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
