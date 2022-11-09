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
	Result string `json:"result"`
	Image  string `json:"image"`
}

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

func recognition(image string) (string, error) {
	dto := RecognitionDto{Image: image}

	reqBody, parseErr := json.Marshal(dto)

	if parseErr != nil {
		return "", parseErr
	}

	if resp, err := http.Post(
		fmt.Sprintf("%s/recognition/binary", APIHost),
		"application/json",
		bytes.NewBuffer(reqBody),
	); err != nil {
		return "", err
	} else {
		defer func() { _ = resp.Body.Close() }()

		body, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			return "", err
		}

		if resp.StatusCode != http.StatusOK {
			return "", errors.New("bad request")
		}

		var bodyDto RecognitionResponseDto

		if err = json.Unmarshal(body, &bodyDto); err != nil {
			return "", err
		}

		if bodyDto.Result != "recognized" {
			return "", errors.New("not recognized")
		}

		return bodyDto.Image, nil
	}
}
