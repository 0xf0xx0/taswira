// auth wrapper for forgejo instances
package taswira

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path"
)

type forgejoUserResponse struct {
	Active        bool   `json:"active"`
	IsAdmin       bool   `json:"is_admin"`
	Login         string `json:"login"`
	ProhibitLogin bool   `json:"prohibit_login"`
	Pronouns      string `json:"pronouns"`
	Restricted    bool   `json:"restricted"`
}

type ForgejoAuth struct {
	Instance string
}

func (fa *ForgejoAuth) IsAlive() {
	res, err := http.Get(path.Join(fa.Instance, "/api/v1/version"))
	if err != nil {
		log.Fatalln(err)
	}
	if res.StatusCode != http.StatusOK {
		log.Fatalf("error from backend forgejo: %s\n", res.Status)
	}
}

func (fa *ForgejoAuth) Authenticate(username, token string, w http.ResponseWriter) (ok bool) {
	req, _ := http.NewRequest("GET", fa.Instance+"/api/v1/user", nil)
	req.Header.Add("Authorization", "token "+token)
	setUA(req)

	res, err := authHttpClient.Do(req)
	if err != nil {
		log.Printf("error sending auth request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte{})
		return false
	}
	if res.StatusCode != http.StatusOK {
		log.Printf("error verifying user %s: %s", username, res.Status)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte{})
		return false
	}
	body, _ := io.ReadAll(res.Body)
	b := &forgejoUserResponse{}
	if json.Unmarshal(body, b) != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte{})
		return false
	}

	if b.ProhibitLogin || b.Restricted || !b.Active {
		/// TODO: better logline
		log.Printf("%s failed login\n")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte{})
		return false
	}
	return true
}
