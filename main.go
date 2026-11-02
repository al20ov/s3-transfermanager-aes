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
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// "github.com/goforj/godump"

var bucket string = "filestash"
var key string = "hellofile"
var contentType string = "text/plain"

var secretHex string = "6368616e676520746869732070617373" // "change this pass"

const maxBodySize int64 = 32 << 24

type AppState struct {
	transfermanagerClient *transfermanager.Client
	s3Client              *s3.Client
	ctx                   *context.Context
}

func main() {
	ctx := context.Background()
	sdkConfig, err := config.LoadDefaultConfig(ctx)

	server := http.NewServeMux()

	if err != nil {
		panic(err)
	}

	s3Client := s3.NewFromConfig(sdkConfig)
	transfermanagerClient := transfermanager.New(s3Client)

	appState := AppState{
		transfermanagerClient: transfermanagerClient,
		s3Client:              s3Client,
		ctx:                   &ctx,
	}

	server.HandleFunc("POST /{path...}", appState.PutObjectHandler)

	http.ListenAndServe("0.0.0.0:8080", server)
}

func (s *AppState) PutObjectHandler(w http.ResponseWriter, request *http.Request) {
	path := request.PathValue("path")

	request.Body = http.MaxBytesReader(w, request.Body, maxBodySize)

	if err := request.ParseMultipartForm(maxBodySize); err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, _, err := request.FormFile("file")

	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}

	defer file.Close()
	fullPath := filepath.Join(path, key)
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
	output, err := s.transfermanagerClient.UploadObject(*s.ctx, &transfermanager.UploadObjectInput{
		Bucket:      &bucket,
		Key:         &fullPath,
		ContentType: &contentType,
		Body:        reader,
		Metadata: map[string]string{
			"encryption": "aes-128-ctr",
			"iv":         hex.EncodeToString(iv),
		},
	})
	_ = output

	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Sent file", key, "to", bucket)
	fmt.Fprintln(w, "OK")
}

// To decrypt after the fact: `openssl enc -d -aes-128-ctr -in encrypted_file.bin -out decrypted_file.txt -K <HEX_KEY> -iv <HEX_IV>`
