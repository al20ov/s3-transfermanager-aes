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

var hexSecret string = "3275636b7940737472316b3342424148"

const maxBodySize int64 = 32 << 24

type AppState struct {
	transfermanagerClient *transfermanager.Client
	s3Client *s3.Client
	ctx *context.Context
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
		s3Client: s3Client,
		ctx: &ctx,
	}

	server.HandleFunc("POST /{path...}", appState.PutObjectHandler)

	http.ListenAndServe("0.0.0.0:8080", server)
}

func (s *AppState) PutObjectHandler(w http.ResponseWriter, request *http.Request) {
	// Preparation
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
	// END preparation
	// Encryption stuff
	secret, _ := hex.DecodeString(hexSecret)
	
	block, err := aes.NewCipher(secret)
	if err != nil {
		panic(err)
	}

	iv := make([]byte, aes.BlockSize)
	
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		panic(err)
	}

	stream := cipher.NewCTR(block, iv)
	_ = stream

	// END encryption stuff
	output, err := s.transfermanagerClient.UploadObject(*s.ctx, &transfermanager.UploadObjectInput{
		Bucket: &bucket,
		Key: &fullPath,
		Body: file,
		ContentType: &contentType,
		Metadata: map[string]string{
			"encryption": "aes-ctr",
			"iv": hex.EncodeToString(iv),
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

// To decrypt, `openssl enc -d -aes-256-ctr -K <HEX_KEY> -iv <HEX_IV> -in encrypted_file.bin -out decrypted_file.txt` with key hex-encoded



	// ctx := context.Background()
	// sdkConfig, err := config.LoadDefaultConfig(ctx)
	//
	// if err != nil {
	// 	panic(err)
	// }
	//
	// s3Client := s3.NewFromConfig(sdkConfig)
	//
	// client := transfermanager.New(s3Client)
	//
	// client.UploadObject(ctx, &transfermanager.UploadObjectInput{
	// 	Bucket: &bucket,
	// 	Key: &key,
	// })
