A quick and dirty aws-sdk-go-v2 transfermanager test with AES-CTR encryption.

## Running

```sh
go get .
BUCKET_NAME=<your S3 bucket> go run .
```

If you're using R2 and not S3, run it with:
```sh
BUCKET_NAME=<your S3 bucket> AWS_REGION=auto AWS_ENDPOINT_URL=https://<your cloudflare acct ID>.r2.cloudflarestorage.com go run .
```

## Making requests

Then, you can start making requests with `curl`, here I'm sending plaintext
which will get encrypted and stored on the bucket as ciphertext:

```sh
curl -X POST -F "file=@plaintext.txt" http://localhost:8080/testdir/ciphertext.txt
```

Then, to download it back decrypted:

```sh
curl http://localhost:8080/testdir/ciphertext.txt
```

## Notes for me

Objects are stored with a random IV, but that IV is stored in the file's
metadata in object storage so it can be used to decrypt the file back to
cleartext given the original key.

To decrypt a file with openssl instead, use:

```sh
openssl enc -d -aes-128-ctr -in ciphertext.txt -out cleartext.txt -K <hex-encoded key> -iv <hex-encoded iv>
```
