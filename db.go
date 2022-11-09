package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"gopkg.in/guregu/null.v4"
	"os"
	"time"
)

func getEnvOrDefaultValue(envKey string, defaultValue string) string {
	envValue := os.Getenv(envKey)
	if envValue != "" {
		return envValue
	}
	return defaultValue
}

type AttachedDevice struct {
	bun.BaseModel `bun:"table:attached_input_devices,alias:aid"`

	ID         int64     `bun:"id"`
	DeviceName string    `bun:"device_name"`
	DeviceCode string    `bun:"device_code"`
	AuthToken  uuid.UUID `bun:"auth_token"`
	ValidUntil time.Time `bun:"valid_until"`
}

type CaptureSession struct {
	bun.BaseModel `bun:"table:capture_sessions,alias:cs"`

	ID          int64     `bun:"id"`
	SessionType string    `bun:"session_type"`
	EndTime     null.Time `bun:"end_time"`
}

func getAttachedDevice(deviceCode string, authToken uuid.UUID) (AttachedDevice, error) {
	var device AttachedDevice

	err := db.NewSelect().
		Model(&device).
		Where("aid.device_code = ? AND aid.auth_token = ? AND aid.valid_until > now()", deviceCode, authToken).
		Scan(context.Background())

	return device, err
}

func getCaptureSession(deviceId string) (CaptureSession, error) {
	var session CaptureSession

	err := db.NewSelect().
		Model(&session).
		Join("JOIN attached_input_devices AS aid ON aid.id = cs.attached_device_id").
		Where("aid.device_code = ? AND end_time IS NULL", deviceId).
		Scan(context.Background())

	return session, err
}

func GetBunDb() *bun.DB {
	DbHost := getEnvOrDefaultValue("DB_HOST", "localhost")
	DbPort := getEnvOrDefaultValue("DB_PORT", "5432")
	DbUser := getEnvOrDefaultValue("DB_USER", "postgres")
	DbPassword := getEnvOrDefaultValue("DB_PASSWORD", "postgres")
	DbName := getEnvOrDefaultValue("DB_NAME", "person_recognition_system_database")

	db := sql.OpenDB(pgdriver.NewConnector(
		pgdriver.WithAddr(fmt.Sprintf("%v:%v", DbHost, DbPort)),
		pgdriver.WithUser(DbUser),
		pgdriver.WithPassword(DbPassword),
		pgdriver.WithDatabase(DbName),
		pgdriver.WithTLSConfig(nil),
		pgdriver.WithApplicationName("socket-server"),
	))

	maxOpenConns := 20
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)

	return bun.NewDB(db, pgdialect.New())
}
