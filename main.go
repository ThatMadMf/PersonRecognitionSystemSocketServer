package main

import (
	"bytes"
	"encoding/base64"
	"errors"
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

var connections = make([]Socket, 0)

func main() {
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/", handleWebSocket)
	router.Use(mux.CORSMethodMiddleware(router))
	router.Use(jwtMiddleware)
	log.Fatalln(http.ListenAndServe(":5005", router))
}

const InputDevice = "INPUT_DEVICE"
const Admin = "ADMIN"

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
	userId            string
	deviceId          string
	authorizationUUID uuid.UUID
}

type Socket struct {
	conn    *websocket.Conn
	context SocketContext
	id      uuid.UUID
	rooms   []string
}

func (s Socket) hasRoom(room string) bool {
	for _, r := range s.rooms {
		if r == room {
			return true
		}
	}

	return false
}

type DeviceAuthorizationDto struct {
	DeviceId  string `json:"deviceId"`
	AuthToken string `json:"authToken"`
}

type FaceCaptureFrameDto struct {
	Image string `json:"image"`
}

type FrameCapturedDto struct {
	DeviceId string `json:"deviceId"`
	Image    string `json:"image"`
}

func removeSocket(socket Socket) {
	for i, s := range connections {
		if s.id == socket.id {
			connections[i] = connections[len(connections)-1]
			connections = connections[:len(connections)-1]

			return
		}
	}
}

func sendToRoom(room string, event interface{}) {
	for _, s := range connections {
		if s.hasRoom(room) {
			if err := s.conn.WriteJSON(&event); err != nil {
				log.Printf("Could not write to socket %v in %v room: %v", s.id, room, err)
			}
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	var conn *websocket.Conn

	if upgradedConn, err := upgrader.Upgrade(w, r, nil); err != nil {
		log.Printf("Could not upgrade connection: %v", err.Error())

		return
	} else {
		conn = upgradedConn
	}

	socket := Socket{conn: conn, id: uuid.New()}

	if userID := r.Header.Get("ID"); userID != "" {
		socket.context.connType = Admin
		socket.context.userId = userID
		socket.rooms = []string{"admin"}

		log.Print("Admin socket connected")
	} else {
		log.Print("Socket connected")
	}

	connections = append(connections, socket)

	defer func() {
		_ = conn.Close()

		removeSocket(socket)

		log.Print("Socket disconnected")
	}()

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
		case "start-capture-session":
			// Error if session for auth-token exists
			// Create & return session id if ok

			if socket.context.connType != InputDevice {
				err = errors.New("not authorized as input device")
			}

			response, err = startCaptureSession()
		case "face-capture-frame":
			if socket.context.connType != InputDevice {
				err = errors.New("not authorized as input device")
			}

			response, err = faceCaptureFrame(socket, event)
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
		connType:          InputDevice,
		deviceId:          dto.DeviceId,
		authorizationUUID: token,
	}

	return EventResponse{
		Uuid:    event.Uuid,
		Command: event.Command,
		Result:  "success",
	}, nil
}

func faceCaptureFrame(socket Socket, event Event) (EventResponse, error) {
	var dto FaceCaptureFrameDto
	if err := mapstructure.Decode(event.Data, &dto); err != nil {
		return EventResponse{}, err
	}

	sendToRoom("admin", Event{
		Uuid:    uuid.New(),
		Command: "frame-captured",
		Data: FrameCapturedDto{
			DeviceId: socket.context.deviceId,
			Image:    dto.Image,
		},
	})

	return EventResponse{
		Uuid:    event.Uuid,
		Command: event.Command,
		Result:  "success",
	}, nil
}

func startCaptureSession() (EventResponse, error) {
	panic("not implemented")
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
