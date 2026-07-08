package admin

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func generateTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestUploadAvatar_JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/avatar", UploadAvatar)

	pngBytes := generateTestPNG()
	b64Str := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	reqBody := bytes.NewBufferString(`{"image":"` + b64Str + `"}`)
	req, _ := http.NewRequest("POST", "/api/avatar", reqBody)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Clean up
	_ = os.Remove(avatarPath)
}

func TestUploadAvatar_Multipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/avatar", UploadAvatar)

	pngBytes := generateTestPNG()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("image", "avatar.png")
	assert.NoError(t, err)
	_, err = part.Write(pngBytes)
	assert.NoError(t, err)
	writer.Close()

	req, _ := http.NewRequest("POST", "/api/avatar", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Clean up
	_ = os.Remove(avatarPath)
}
