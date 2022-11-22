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

var connections = make([]*Socket, 0)
var db = GetBunDb()

func main() {
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/", handleWebSocket)
	router.Use(mux.CORSMethodMiddleware(router))
	router.Use(jwtMiddleware)
	log.Fatalln(http.ListenAndServe(":5005", router))
}

const InputDevice = "INPUT_DEVICE"
const Admin = "ADMIN"

const AuthorizationResultEvent = "authorization-result"

const FaceNotDetectedResult = "face not detected"
const NotRecognizedResult = "not recognized"
const RecognizedResult = "recognized"

const SessionCapturesLimit = 10

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
	deviceName        string
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

type DeviceAuthorizedDto struct {
	DeviceId   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
}

type FaceCaptureFrameDto struct {
	Image string `json:"image"`
}

type FrameCapturedDto struct {
	DeviceId string `json:"deviceId"`
	Image    string `json:"image"`
}

type AuthorizationDto struct {
	Result  string `json:"result"`
	Message string `json:"message"`
	Token   string `json:"token"`
}

type DeviceDisconnectedDto struct {
	DeviceId string `json:"deviceId"`
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

	connections = append(connections, &socket)

	defer func() {
		_ = conn.Close()

		if socket.context.connType == InputDevice {
			sendToRoom(
				"admin",
				Event{
					Uuid:    uuid.New(),
					Command: "device-disconnected",
					Data:    DeviceDisconnectedDto{DeviceId: socket.context.deviceId},
				})
		}

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
		case "get-devices":
			if socket.context.connType != Admin {
				err = errors.New("not authorized as admin")

				break
			}

			response, err = getDevices(event)
		case "start-capture-session":
			// Error if session for auth-token exists
			// Create & return session id if ok

			if socket.context.connType != InputDevice {
				err = errors.New("not authorized as input device")

				break
			}

			response, err = startCaptureSession(socket, event)
		case "face-capture-frame":
			if socket.context.connType != InputDevice {
				err = errors.New("not authorized as input device")

				break
			}

			response, err = faceCaptureFrame(socket, event)
		default:
			err = errors.New(fmt.Sprintf("unknown command: %v", event.Command))
		}

		if err != nil {
			log.Printf(
				"Error during event %s: %v",
				event.Command,
				err.Error(),
			)
			response = EventResponse{
				Uuid:    event.Uuid,
				Command: event.Command,
				Result:  "error",
				Data:    err.Error(),
			}
		} else {
			log.Printf(
				"Successfuly handled event %s",
				event.Command,
			)
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

	if device, deviceErr := getAttachedDevice(dto.DeviceId, token); deviceErr != nil {
		return EventResponse{}, deviceErr
	} else {
		sendToRoom("admin", Event{
			Uuid:    uuid.New(),
			Command: "device-authorized",
			Data: DeviceAuthorizedDto{
				DeviceId:   dto.DeviceId,
				DeviceName: device.DeviceName,
			},
		})

		socket.context = SocketContext{
			connType:          InputDevice,
			deviceId:          dto.DeviceId,
			deviceName:        device.DeviceName,
			authorizationUUID: token,
		}
	}

	return EventResponse{
		Uuid:    event.Uuid,
		Command: event.Command,
		Result:  "success",
	}, nil
}

func getDevices(event Event) (EventResponse, error) {
	devices := make([]DeviceAuthorizedDto, 0)

	for _, c := range connections {
		if c.context.connType == InputDevice {
			devices = append(devices, DeviceAuthorizedDto{
				DeviceId:   c.context.deviceId,
				DeviceName: c.context.deviceName,
			})
		}
	}

	return EventResponse{
		Uuid:    event.Uuid,
		Command: event.Command,
		Result:  "success",
		Data:    devices,
	}, nil
}

func faceCaptureFrame(socket Socket, event Event) (EventResponse, error) {
	var dto FaceCaptureFrameDto
	var image string

	if err := mapstructure.Decode(event.Data, &dto); err != nil {
		return EventResponse{}, err
	}

	image = dto.Image

	if session, err := getCaptureSession(socket.context.deviceId); err == nil {
		if count, countErr := getSessionFramesCount(session.ID); countErr != nil {
			log.Printf("Could not count session frames: %v", countErr)
		} else if count > SessionCapturesLimit {
			if token, authorizationErr := completeCaptureSession(session.ID); authorizationErr != nil {
				log.Printf("Could not complete session: %v", authorizationErr)

				return EventResponse{
					Uuid:    uuid.New(),
					Command: AuthorizationResultEvent,
					Result:  "error",
					Data: AuthorizationDto{
						Result:  "error",
						Message: authorizationErr.Error(),
					},
				}, nil
			} else {
				return EventResponse{
					Uuid:    uuid.New(),
					Command: AuthorizationResultEvent,
					Result:  "success",
					Data: AuthorizationDto{
						Token: token,
					},
				}, nil
			}
		}

		response, err := recognition(dto.Image)
		if err != nil {
			return EventResponse{}, err
		}
		switch response.Result {
		case FaceNotDetectedMessage:
			log.Println("not recognized")
		case NotRecognizedResult:
			if err = createSessionFrame(SessionFrame{
				FrameDetails: response.Result,
				Timestamp:    time.Now(),
				SessionID:    session.ID,
			}); err != nil {
				log.Printf("Could not create session frame record: %v", err)
			}

		case RecognizedResult:
			image = response.Image
			if err = createSessionFrame(SessionFrame{
				FrameDetails: response.Result,
				Timestamp:    time.Now(),
				SessionID:    session.ID,
				Users: []*SessionFrameUser{
					{
						Value:  response.Confidence,
						UserID: response.UserID,
					},
				},
			}); err != nil {
				log.Printf("Could not create session frame record: %v", err)
			}
		}

	}

	sendToRoom("admin", Event{
		Uuid:    uuid.New(),
		Command: "frame-captured",
		Data: FrameCapturedDto{
			DeviceId: socket.context.deviceId,
			Image:    image,
		},
	})

	return EventResponse{
		Uuid:    event.Uuid,
		Command: event.Command,
		Result:  "success",
	}, nil
}

func startCaptureSession(socket Socket, event Event) (EventResponse, error) {
	if _, err := getCaptureSession(socket.context.deviceId); err == nil {
		return EventResponse{}, errors.New("capture session already exists")
	}

	if err := createCaptureSession(CreateCaptureSessionDto{
		AttachedDeviceToken: socket.context.authorizationUUID,
	}); err != nil {
		return EventResponse{}, err
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
