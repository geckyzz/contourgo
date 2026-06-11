package crypto

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/fernet/fernet-go"
)

func DecryptAndDecompress(ciphertext []byte, keyStr string) ([]byte, error) {
	k, err := fernet.DecodeKey(keyStr)
	if err != nil {
		return nil, err
	}

	keys := []*fernet.Key{k}
	msg := fernet.VerifyAndDecrypt(ciphertext, 0, keys)
	if msg == nil {
		return nil, io.ErrUnexpectedEOF
	}

	r, err := gzip.NewReader(bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}
