package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// Establecer un límite de carga de 1 GB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	// Extraer el videoID de los parámetros de la URL y parsearlo como UUID
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Autenticar al usuario para obtener el userID
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// Obtener los metadatos del video de la base de datos
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}

	// Verificar si el usuario autenticado es el propietario del video
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	// Parsear el archivo de video subido desde los datos del formulario
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get file from form", err)
		return
	}
	defer file.Close()

	// Validar el archivo subido para asegurarse de que es un video MP4
	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil || mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}

	// Guardar el archivo subido en un archivo temporal en el disco
	tempFile, err := os.CreateTemp("", "tubely-upload-*.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to save file", err)
		return
	}

	// Restablecer el puntero del archivo temporal al principio
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to seek temp file", err)
		return
	}

	// Generar un nombre de archivo aleatorio
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to generate random bytes", err)
		return
	}
	randomFileName := base64.RawURLEncoding.EncodeToString(randomBytes) + ".mp4"

	// Subir el objeto a S3 usando PutObject
	_, err = cfg.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &randomFileName,
		Body:        tempFile,
		ContentType: &mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to upload to S3", err)
		return
	}

	// Actualizar la VideoURL del registro del video en la base de datos
	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, randomFileName)
	video.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	// Responder con los metadatos actualizados del video en formato JSON
	respondWithJSON(w, http.StatusOK, video)
}
