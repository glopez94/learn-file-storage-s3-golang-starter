package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

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
	mediaType := header.Header.Get("Content-Type")

	// Leer todos los datos de la imagen en un slice de bytes
	imageData, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to read file", err)
		return
	}

	// Convertir los datos de la imagen a una cadena base64
	base64Data := base64.StdEncoding.EncodeToString(imageData)
	// Crear una URL de datos con el tipo de medio y los datos codificados en base64
	dataURL := fmt.Sprintf("data:%s;base64,%s", mediaType, base64Data)

	// Obtener los metadatos del video de la base de datos SQLite
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

	// Actualizar la base de datos con la nueva URL de la miniatura
	video.ThumbnailURL = &dataURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	// Responder con los metadatos actualizados del video en formato JSON
	respondWithJSON(w, http.StatusOK, video)
}
