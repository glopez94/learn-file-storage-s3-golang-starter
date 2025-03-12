package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// Configurar el límite de memoria
	const maxMemory = 10 << 20 // 10MB

	// Parsear los datos del formulario
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form", err)
		return
	}

	// Obtener los datos de la imagen del formulario
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get file from form", err)
		return
	}
	defer file.Close()

	// Obtener el tipo de medio del encabezado Content-Type del archivo
	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}

	// Verificar si el tipo de medio es permitido
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Unsupported media type", nil)
		return
	}

	extensions, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(extensions) == 0 {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}
	fileExtension := extensions[0]

	// Generar un nombre de archivo aleatorio
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to generate random bytes", err)
		return
	}
	randomFileName := base64.RawURLEncoding.EncodeToString(randomBytes) + fileExtension

	// Crear una ruta de archivo única
	filePath := filepath.Join(cfg.assetsRoot, randomFileName)

	// Crear el nuevo archivo
	outFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file", err)
		return
	}
	defer outFile.Close()

	// Copiar el contenido del multipart.File al nuevo archivo en el disco
	_, err = io.Copy(outFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to save file", err)
		return
	}

	// Actualizar la base de datos con la nueva URL de la miniatura
	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, randomFileName)
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	video.ThumbnailURL = &thumbnailURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	// Responder con los metadatos actualizados del video en formato JSON
	respondWithJSON(w, http.StatusOK, video)
}
