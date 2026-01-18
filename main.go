package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/zeebo/xxh3"
	_ "golang.org/x/image/webp"
)

type forgejoUserResponse struct {
	Active        bool   `json:"active"`
	IsAdmin       bool   `json:"is_admin"`
	Login         string `json:"login"`
	ProhibitLogin bool   `json:"prohibit_login"`
	Pronouns      string `json:"pronouns"`
	Restricted    bool   `json:"restricted"`
}

type response struct {
	Message string `json:"message"`
	Url     string `json:"url,omitempty"`
}
type errorResponse struct {
	Message string `json:"error"`
}

const version = "1.0.0"

var (
	USER_AGENT  = fmt.Sprintf("taswira/v%s", version) /// TODO: finish name
	INSTANCE    = os.Getenv("INSTANCE")
	IMG_ROOT    = os.Getenv("IMG_ROOT")
	imgroot     *os.Root
	SUBPATH     = os.Getenv("SUBPATH")
	LISTEN_PORT = func() int {
		env := os.Getenv("LISTEN_PORT")
		if env == "" {
			return 6969
		}
		i, err := strconv.Atoi(env)
		if err != nil {
			log.Fatalln(err)
		}
		/// clamp to port range
		/// yes this overflows, no we dont care
		/// config your shit right lmao
		return int(uint16(i))
	}()
)

func main() {
	var err error
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	if INSTANCE == "" {
		log.Fatalln("no forgejo instance set for auth")
	}
	if SUBPATH != "" {
		SUBPATH += "/"
	}
	if IMG_ROOT == "" {
		IMG_ROOT = "./img"
	}
	imgroot, err = os.OpenRoot(IMG_ROOT)
	if err != nil {
		log.Fatalln(err)
	}
	http.HandleFunc("/", mainHandler)

	go func() {
		log.Printf("listening on http://localhost:%d\n", LISTEN_PORT)
		log.Fatal(http.ListenAndServe(fmt.Sprintf("localhost:%d", LISTEN_PORT), nil))
	}()

	// wait for exit
	<-sigs
}

func authUser(instance, token, username string, w http.ResponseWriter) (*forgejoUserResponse, bool) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", instance+"/api/v1/user", nil)
	req.Header.Add("Authorization", "token "+token)
	req.Header.Add("User-Agent", USER_AGENT)

	res, err := client.Do(req)
	if err != nil {
		log.Fatalln(err)
		w.Write([]byte{})
		return nil, false
	}
	if res.StatusCode != http.StatusOK {
		println(username, token)
		w.Write([]byte{})
		log.Fatalf("error verifying user: %s", res.Status)
		return nil, false
	}
	body, _ := io.ReadAll(res.Body)
	b := &forgejoUserResponse{}
	if json.Unmarshal(body, b) != nil {
		log.Fatalln(err)
		w.Write([]byte{})
		return nil, false
	}
	return b, true
}

// handles auth and delegates to method handlers
func mainHandler(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)

	scheme := r.Header.Get("X-Forwarded-Proto")
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Header.Get("Host")
	}
	if scheme == "" {
		log.Fatal("X-Forwarded-Proto header not set")
	}
	if host == "" {
		log.Fatal("X-Forwarded-Host or Host header not set")
	}
	urlPfx := fmt.Sprintf("%s://%s/%s", scheme, host, SUBPATH)

	var handler func(urlPfx string, r *http.Request, username string, w http.ResponseWriter) bool
	switch r.Method {
	case "POST":
		handler = postHandler
	case "DELETE":
		handler = deleteHandler
	default:
		return
	}

	username, token, ok := r.BasicAuth()
	if !ok {
		return
	}

	forgejoRes, ok := authUser(INSTANCE, token, username, w)
	if !ok {
		return
	}

	if username != forgejoRes.Login || forgejoRes.ProhibitLogin || forgejoRes.Restricted || !forgejoRes.Active {
		log.Printf("failed login: %s\n", username)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte{})
		return
	}
	w.Header().Add("Server", USER_AGENT)
	ok = handler(urlPfx, r, username, w)
	if !ok {
		return
	}
}

func postHandler(urlPfx string, r *http.Request, username string, w http.ResponseWriter) bool {
	uploadBody, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading image from %s: %s\n", username, err)
		e := &errorResponse{
			Message: fmt.Sprintf("error reading image: %s", err),
		}
		writeError(e, w, http.StatusUnprocessableEntity)
		return false
	}
	if len(uploadBody) > 1024*1024*256 { /// 256MiB
		e := &errorResponse{
			Message: "image too large (>256MiB)",
		}
		writeError(e, w, http.StatusRequestEntityTooLarge)
	}
	decodedImg, _, err := image.Decode(bytes.NewReader(uploadBody))
	if err != nil {
		log.Printf("%q\n", uploadBody[:80])
		log.Printf("error decoding image from %s: %s\n", username, err)
		e := &errorResponse{
			Message: fmt.Sprintf("error decoding image: %s", err),
		}
		writeError(e, w, http.StatusUnsupportedMediaType)
		return false
	}

	/// check for dupe before processing further
	hash := xxh3.Hash128(uploadBody).Bytes()
	filename := hex.EncodeToString(hash[:]) + ".png"
	url := urlPfx + filename
	if checkIfImageExists(filename) {
		e := &errorResponse{
			Message: "duplicate image",
		}
		writeError(e, w, http.StatusConflict)
		return false
	}

	nrgbaImg, ok := decodedImg.(*image.NRGBA)
	if !ok {
		err := errors.New(fmt.Sprintf("error decoding image from %s: %s\n", username, err))
		log.Println(err)
		e := &errorResponse{
			Message: fmt.Sprintf("error decoding image: %s", err),
		}
		writeError(e, w, http.StatusUnprocessableEntity)
		return false
	}

	encodedImg := bytes.NewBuffer(make([]byte, 0, len(uploadBody)))
	err = png.Encode(encodedImg, nrgbaImg)
	if err != nil {
		log.Printf("error encoding image from %s: %s\n", username, err)
		e := &errorResponse{
			Message: fmt.Sprintf("error encoding image: %s", err),
		}
		writeError(e, w, http.StatusUnprocessableEntity)
		return false
	}

	/// verify the hash after metadata removal
	hash = xxh3.Hash128(encodedImg.Bytes()).Bytes()
	filename = hex.EncodeToString(hash[:]) + ".png"
	url = urlPfx + filename
	if checkIfImageExists(filename) {
		e := &errorResponse{
			Message: "duplicate image",
		}
		writeError(e, w, http.StatusConflict)
		return false
	}

	/// write
	out, err := imgroot.Create(filename)
	if err != nil {
		e := &errorResponse{
			Message: fmt.Sprintf("error writing %s", filename),
		}
		log.Printf("error creating image file for %s: %s\n", username, err)
		writeError(e, w, http.StatusInternalServerError)
		return false
	}
	_, err = encodedImg.WriteTo(out)
	if err != nil {
		e := &errorResponse{
			Message: fmt.Sprintf("error writing %s", filename),
		}
		log.Printf("error writing image file for %s: %s\n", username, err)
		writeError(e, w, http.StatusInternalServerError)
		return false
	}

	log.Printf("successful upload from %s: %s", username, filename)
	m := &response{
		Message: "ok",
		Url:     url,
	}
	writeResponse(m, w)
	return true
}
func deleteHandler(_ string, r *http.Request, username string, w http.ResponseWriter) bool {
	deletehash := r.URL.Query().Get("hash")
	filename := deletehash + ".png"
	if !checkIfImageExists(filename) {
		e := &errorResponse{
			Message: fmt.Sprintf("nonexistent image: %s", filename),
		}
		writeError(e, w, http.StatusNotFound)
		return false
	}
	if err := imgroot.Remove(filename); err != nil {
		e := &errorResponse{
			Message: fmt.Sprintf("error deleting %s: %s", filename, err),
		}
		writeError(e, w, http.StatusNotFound)
		return false
	}
	log.Printf("successful deletion from %s: %s", username, filename)
	m := &response{
		Message: "ok",
	}
	writeResponse(m, w)
	return true
}

func setHeaders(w http.ResponseWriter) {
	w.Header()["Date"] = nil
	w.Header().Set("Content-Type", "application/json")
}

func checkIfImageExists(path string) bool {
	_, err := imgroot.Stat(path)
	if err == nil {
		return true
	}
	return false
}

func writeResponse(res *response, w http.ResponseWriter) {
	b, _ := json.Marshal(res)
	w.Write(b)
}

func writeError(err *errorResponse, w http.ResponseWriter, statusCode int) {
	b, _ := json.Marshal(err)
	w.WriteHeader(statusCode)
	w.Write(b)
}
