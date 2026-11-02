# s3-transfermanager-aes

A quick and dirty aws-sdk-go-v2 transfermanager "gateway" with streaming
AES-CTR encryption.

By streaming I mean it doesn't buffer the entire file to encrypt/decrypt it, it
does it on the fly since counter mode is a streaming cipher but I think
transfermanager might buffer some stuff here or there.

> WARNING: This is just raw AES-CTR with no AEAD so there's no data integrity
> guarantee, but object storage providers rarely flip random bits in your files
> so it's probably fine.

> AES-CTR and other modes of operation explained:
> https://www.youtube.com/watch?v=Rk0NIQfEXBA

## Running

```sh
go get .
BUCKET_NAME=<your S3 bucket> go run .
```

If you're using R2 and not S3, run it with:
```sh
BUCKET_NAME=<your R2 bucket> \
AWS_REGION=auto \
AWS_ENDPOINT_URL=https://<your cloudflare acct ID>.r2.cloudflarestorage.com \
go run .
```

## Making requests

Then, you can start making requests with `curl`, here I'm sending plaintext
which will get encrypted and stored on the bucket as ciphertext:

```sh
curl -X POST --data-binary @plaintext.txt http://localhost:8080/testdir/ciphertext.txt
```

Then, to download it back decrypted:

```sh
curl http://localhost:8080/testdir/ciphertext.txt > plaintext.txt
```

## Notes for me

Objects are encrypted with a random IV which is then stored in the object's
metadata on S3 so it can be used to decrypt the file back to cleartext given
the original key.

To decrypt a file with openssl instead, use:

```sh
openssl enc -d -aes-128-ctr -in ciphertext.txt -out cleartext.txt -K <hex-encoded key> -iv <hex-encoded iv>
```

Also, the only valid block size for AES is 128 bits, the IV (nonce and counter)
needs to be 128 bits as well but the key can be 128, 192 or 256 bits long which
also affects the number of rounds.

TODO: replace mime.TypeByExtension with http.DetectContentType?
