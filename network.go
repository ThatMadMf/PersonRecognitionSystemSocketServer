package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io/ioutil"
	"log"
	"net/http"
)

var APIHost = getEnvOrDefaultValue("API_HOST", "http://localhost:8000/api")

type CreateCaptureSessionDto struct {
	AttachedDeviceToken uuid.UUID `json:"attachedDeviceToken"`
}

type RecognitionDto struct {
	Image string `json:"image"`
}

type RecognitionResponseDto struct {
	Result     string  `json:"result"`
	UserID     int64   `json:"userId"`
	Confidence float64 `json:"confidence"`
	Image      string  `json:"image"`
}

const FaceNotDetectedMessage = "face not detected"

func createCaptureSession(dto CreateCaptureSessionDto) error {
	reqBody, parseErr := json.Marshal(dto)

	if parseErr != nil {
		return parseErr
	}

	if resp, err := http.Post(
		fmt.Sprintf("%s/capture-sessions", APIHost),
		"application/json",
		bytes.NewBuffer(reqBody),
	); err != nil {
		return err
	} else {
		defer func() { _ = resp.Body.Close() }()

		body, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			return err
		}

		log.Print(string(body))
	}

	return nil
}

//func completeCaptureSession(id int64) (string, error) {
//
//}

func recognition(image string) (RecognitionResponseDto, error) {
	dto := RecognitionDto{Image: image}

	reqBody, parseErr := json.Marshal(dto)

	if parseErr != nil {
		return RecognitionResponseDto{}, parseErr
	}

	if resp, err := http.Post(
		fmt.Sprintf("%s/recognition/binary", APIHost),
		"application/json",
		bytes.NewBuffer(reqBody),
	); err != nil {
		return RecognitionResponseDto{}, err
	} else {
		defer func() { _ = resp.Body.Close() }()

		body, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			return RecognitionResponseDto{}, err
		}

		var bodyDto RecognitionResponseDto

		if err = json.Unmarshal(body, &bodyDto); err != nil {
			return RecognitionResponseDto{}, err
		}

		if resp.StatusCode != http.StatusOK {
			return RecognitionResponseDto{}, errors.New("bad request")
		}

		return bodyDto, nil
	}
}
