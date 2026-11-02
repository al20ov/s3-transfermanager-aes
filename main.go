package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// "github.com/goforj/godump"

var bucket string

var contentType string = "text/plain"

var secretHex string = "6368616e676520746869732070617373" // "change this pass"

const maxBodySize int64 = 32 << 28 // 8GiB

type AppState struct {
	transfermanagerClient *transfermanager.Client
	ctx                   *context.Context
}

func main() {
	ctx := context.Background()

	bucket = os.Getenv("BUCKET_NAME")
	if bucket == "" {
		panic("BUCKET_NAME is required")
	}

	sdkConfig, err := config.LoadDefaultConfig(ctx)

	server := http.NewServeMux()

	if err != nil {
		panic(err)
	}

	s3Client := s3.NewFromConfig(sdkConfig)
	transfermanagerClient := transfermanager.New(s3Client)

	appState := AppState{
		transfermanagerClient: transfermanagerClient,
		ctx:                   &ctx,
	}

	server.HandleFunc("POST /{path...}", appState.PutObjectHandler)
	server.HandleFunc("PUT /{path...}", appState.PutObjectHandler)

	server.HandleFunc("GET /{path...}", appState.GetObjectHandler)

	http.ListenAndServe("0.0.0.0:8080", server)
}

func (s *AppState) PutObjectHandler(w http.ResponseWriter, request *http.Request) {
	path := request.PathValue("path")

	request.Body = http.MaxBytesReader(w, request.Body, maxBodySize)

	file := request.Body

	defer file.Close()

	// key comes from the last segment of the path
	key := strings.TrimPrefix(path, "/")
	fullPath := path
	// aes stuff

	secret, _ := hex.DecodeString(secretHex)
	block, err := aes.NewCipher(secret)

	if err != nil {
		panic(err)
	}

	iv := make([]byte, aes.BlockSize)

	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}

	stream := cipher.NewCTR(block, iv)

	reader := &cipher.StreamReader{S: stream, R: file}

	// END aes stuff
	_, err = s.transfermanagerClient.UploadObject(*s.ctx, &transfermanager.UploadObjectInput{
		Bucket:      &bucket,
		Key:         &fullPath,
		ContentType: &contentType,
		Body:        reader,
		Metadata: map[string]string{
			"encryption": "aes-128-ctr",
			"iv":         hex.EncodeToString(iv),
		},
	})

	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Sent file", key, "to", bucket)
	fmt.Fprintln(w, "OK")
}

func (s *AppState) GetObjectHandler(w http.ResponseWriter, request *http.Request) {
	path := request.PathValue("path")

	if strings.HasSuffix(path, "/") {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}
	filename := strings.TrimPrefix(path, "/")

	output, err := s.transfermanagerClient.GetObject(*s.ctx, &transfermanager.GetObjectInput{
		Bucket: &bucket,
		Key:    &path,
	})

	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusInternalServerError)
		return
	}

	if output.Body == nil {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}

	// aes decrypt
	secret, _ := hex.DecodeString(secretHex)
	block, err := aes.NewCipher(secret)
	if err != nil {
		panic(err)
	}

	ivHex, ok := output.Metadata["iv"]
	if !ok {
		http.Error(w, "Object has no encryption metadata", http.StatusBadRequest)
		return
	}
	iv, err := hex.DecodeString(ivHex)
	if err != nil || len(iv) != aes.BlockSize {
		http.Error(w, "Invalid encryption metadata", http.StatusBadRequest)
		return
	}

	stream := cipher.NewCTR(block, iv)
	reader := &cipher.StreamReader{S: stream, R: output.Body}

	// END aes decrypt
	contentType := "application/octet-stream"
	if output.ContentType != nil {
		contentType = *output.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))

	_, err = io.Copy(w, reader)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}
}

// To decrypt after the fact: `openssl enc -d -aes-128-ctr -in encrypted_file.bin -out decrypted_file.txt -K <HEX_KEY> -iv <HEX_IV>`
