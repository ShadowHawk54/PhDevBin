package PhDevBin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

type vconfig struct {
	vAPIEndpoint   string
	vAPIKey        string
	statusEndpoint string
	configured     bool
}

var vc vconfig

// Vresult is set by the V API
type Vresult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitmissing"`
	Data    Vagent `json:"data"`
}

// Vagent is set by the V API
type Vagent struct {
	EnlID       EnlID   `json:"enlid"`
	Vlevel      float64 `json:"vlevel"`
	Vpoints     float64 `json:"vpoints"`
	Agent       string  `json:"agent"`
	Level       float64 `json:"level"`
	Quarantine  bool    `json:"quarantine"`
	Active      bool    `json:"active"`
	Blacklisted bool    `json:"blacklisted"`
	Verified    bool    `json:"verified"`
	Flagged     bool    `json:"flagged"`
	Banned      bool    `json:"banned_by_nia"`
	Cellid      string  `json:"cellid"`
}

// SetVEnlOne is called from main() to initialize the config
func SetVEnlOne(w string) {
	Log.Debugf("V.enl.one API Key: %s", w)
	vc.vAPIKey = w
	vc.vAPIEndpoint = "https://v.enl.one/api/v1"
	vc.statusEndpoint = "https://status.enl.one/api/location"
	vc.configured = true
}

// GetvEnlOne is used for templates to determine if V is enabled
func GetvEnlOne() bool {
	return vc.configured
}

// VSearch checks a user at V and populates a Vresult
// gid can be GoogleID, TelegramID or ENL-ID so this should be interface{} instead of GoogleID
func (gid GoogleID) VSearch(res *Vresult) error {
	return vsearch(gid, res)
}

// VSearch checks a user at V and populates a Vresult
func (eid EnlID) VSearch(res *Vresult) error {
	return vsearch(eid, res)
}

// VSearch checks a user at V and populates a Vresult
func (tgid TelegramID) VSearch(res *Vresult) error {
	id := strconv.Itoa(int(tgid))
	return vsearch(id, res)
}

// vsearch stands behind the wraper functions and checks a user at V and populates a Vresult
func vsearch(i interface{}, res *Vresult) error {
	var searchID string
	switch id := i.(type) {
	case GoogleID:
		searchID = id.String()
	case EnlID:
		searchID = id.String()
	case string:
		searchID = id
	default:
		searchID = ""
	}

	if vc.configured == false {
		return errors.New("V API key not configured")
	}
	url := fmt.Sprintf("%s/agent/%s/trust?apikey=%s", vc.vAPIEndpoint, searchID, vc.vAPIKey)
	resp, err := http.Get(url)
	if err != nil {
		Log.Error(err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		Log.Error(err)
		return err
	}

	// Log.Debug(string(body))
	err = json.Unmarshal(body, &res)
	if err != nil {
		Log.Error(err)
		return err
	}
	if res.Status != "ok" {
		err = errors.New(res.Message)
		Log.Info(err)
		return err
	}
	// Log.Debug(res.Data.Agent)
	return nil
}

// VUpdate updates the database to reflect an agent's current status at V.
// It should be called whenever a user logs in via a new service (if appropriate); currently only https does.
func (gid GoogleID) VUpdate(res *Vresult) error {
	if vc.configured == false {
		return errors.New("V API key not configured")
	}

	if res.Status == "ok" && res.Data.Agent != "" {
		Log.Debug("Updating V data for ", res.Data.Agent)
		_, err := db.Exec("UPDATE user SET iname = ?, level = ?, VVerified = ?, VBlacklisted = ?, Vid = ? WHERE gid = ?",
			res.Data.Agent, res.Data.Level, res.Data.Verified, res.Data.Blacklisted, res.Data.EnlID, gid)

		if err != nil {
			Log.Error(err)
			return err
		}
	}
	return nil
}

type statusResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Lon     string `json:"lon"`
	Lat     string `json:"lat"`
	Time    string `json:"time"`
}

// StatusLocation attempts to check for location data from status.enl.one.
// The API documentation is scant, so this does not work.
func (enlID EnlID) StatusLocation() (string, string, error) {
	if vc.configured == false {
		return "", "", errors.New("V API key not configured")
	}
	url := fmt.Sprintf("%s/%s?apikey=%s", vc.statusEndpoint, enlID, vc.vAPIKey)
	resp, err := http.Get(url)
	if err != nil {
		Log.Error(err)
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		Log.Error(err)
		return "", "", err
	}

	var stat statusResponse
	err = json.Unmarshal(body, &stat)
	if err != nil {
		Log.Error(err)
		return "", "", err
	}
	if stat.Status != 0 {
		err = errors.New(fmt.Sprintf("Polling %s returned message: %s", enlID, stat.Message))
		return "", "", err
	}
	return stat.Lat, stat.Lon, nil
}

// StatusLocation attempts to check for location data from status.enl.one.
// The API documentation is scant, so this does not work.
func (gid GoogleID) StatusLocation() (string, string, error) {
	e, _ := gid.EnlID()
	lat, lon, err := e.StatusLocation()
	return lat, lon, err
}

// EnlID returns the V EnlID for a user if it is known.
func (gid GoogleID) EnlID() (EnlID, error) {
	var e EnlID
	err := db.QueryRow("SELECT Vid FROM user WHERE gid = ?", gid).Scan(&e)
	if err != nil {
		Log.Debug(err)
	}
	return e, err
}

func StatusServerPoller() {
	if vc.configured == false {
		Log.Debug("Not polling status.enl.one")
		return
	}

	// loop forever
	Log.Info("Starting status.enl.one Poller")
	for {
		// get list of users who say they use JEAH/RAID
		row, err := db.Query("SELECT gid, Vid FROM user WHERE RAID = 1")
		defer row.Close()
		var gid, vid sql.NullString

		for row.Next() {
			err = row.Scan(&gid, &vid)
			// XXX if the user isn't active on any teams, ignore
			if err != nil {
				Log.Error(err)
				continue
			}
			if vid.Valid == false {
				Log.Info("User requested RAID poll, but has not configured V")
				continue
			}
			e := EnlID(vid.String)
			g := GoogleID(gid.String)
			lat, lon, err := e.StatusLocation()
			if err != nil {
				Log.Error(err)
				continue
			}
			err = g.UserLocation(lat, lon, "status.enl.one")
			if err != nil {
				Log.Error(err)
				continue
			}
		}
		time.Sleep(300 * time.Second)
	}
}
