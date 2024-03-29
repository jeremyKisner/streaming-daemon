package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/jeremyKisner/streaming-daemon/internal/database"
	"github.com/jeremyKisner/streaming-daemon/internal/record"
)

// HandleHealthz returns OK if server is healthy.
func HandleHealthz(w http.ResponseWriter, r *http.Request) {
	fmt.Println("health endpoint called")
	w.Write([]byte("OK"))
}

// HandleAudioInsert processes a POST request containing audio information and file.
// It saves the file to the local filesystem, and inserts info in the database.
func HandleAudioInsert(db database.PostgresConnector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fmt.Println("insert audio called")
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			http.Error(w, "Unable to parse form", http.StatusBadRequest)
			return
		}

		file, handler, err := r.FormFile("audioFile")
		if err != nil {
			http.Error(w, "Error retrieving file from form", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		// initialize record
		record := record.Audio{
			Name:        r.FormValue("name"),
			Artist:      r.FormValue("artist"),
			Album:       r.FormValue("album"),
			Description: r.FormValue("description"),
		}

		wd, err := os.Getwd()
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		filePath := wd + "/audio/" + record.Artist + "_" + record.Album + "_" + record.Name + "_" + handler.Filename
		filePath = strings.ReplaceAll(filePath, " ", "_")
		fmt.Println(record.Name, record.Artist, record.Album, record.Description)
		record.PickupURL = filePath
		destination, err := os.Create(filePath)
		if err != nil {
			fmt.Println("error creating file on server", err)
			http.Error(w, "Error creating file on server", http.StatusInternalServerError)
			return
		}
		defer destination.Close()
		io.Copy(destination, file)
		fmt.Printf("Uploaded file: %s\n", handler.Filename)

		if db.InsertNewAudioRecord(record) {
			w.Write([]byte("Insert Success"))
		} else {
			w.Write([]byte("Insert Failed. Please contact Admin."))
			return
		}
	}
}

// HandleAudioExtraction pulls record info from database.
func HandleAudioExtraction(db database.PostgresConnector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("info endpoint called")
		vars := mux.Vars(r)
		strID := vars["id"]
		id, err := strconv.Atoi(strID)
		if err != nil {
			fmt.Println("error converting int", strID, err)
			http.Error(w, "Error creating file on server", http.StatusInternalServerError)
			return
		}
		res, err := db.ExtractAudioByInternalID(id)
		if err != nil {
			fmt.Println("error extracting data", strID, err)
			http.Error(w, "could not find record", http.StatusInternalServerError)
			return
		}
		w.Write(res)
	}
}

// HandleAudioPlay pulls record from database, and plays file from location.
// It also mutexes a lock on the record and increments the play.
func HandleAudioPlay(db database.PostgresConnector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("play endpoint called")
		vars := mux.Vars(r)
		strID := vars["id"]
		id, err := strconv.Atoi(strID)
		if err != nil {
			fmt.Println("error converting int", strID, err)
			http.Error(w, "bad id", http.StatusForbidden)
			return
		}
		res, err := db.ExtractAudioByInternalID(id)
		if err != nil {
			fmt.Println("error extracting audio record", strID, err)
			http.Error(w, "Had error processing response!", http.StatusNotFound)
			return
		}
		var a record.Audio
		err = json.Unmarshal(res, &a)
		if err != nil {
			fmt.Println("error unmarshal audio record", strID, res, err)
			http.Error(w, "Error unmarshaling data", http.StatusBadRequest)
			return
		}
		file, err := os.Open(a.PickupURL)
		if err != nil {
			fmt.Println("error opening file", strID, a.PickupURL, err)
			http.Error(w, "Error streaming audio", http.StatusNotFound)
			return
		}
		defer file.Close()
		contentType := getContentType(a.PickupURL)
		w.Header().Set("Content-Type", contentType)
		_, err = io.Copy(w, file)
		db.IncrementPlays(a)
		if err != nil {
			fmt.Println("error streaming audio file", strID, err)
			http.Error(w, "Error streaming audio", http.StatusBadRequest)
			return
		}
	}
}

// getContentType determines the file type based on file extension.
func getContentType(filename string) string {
	extension := strings.ToLower(filepath.Ext(filename))
	switch extension {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}
