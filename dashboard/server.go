package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/satori/go.uuid"
	"gosms"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

//reposne structure to /sms
type SMSResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

//response structure to /smsdata/
type SMSDataResponse struct {
	Status   int            `json:"status"`
	Message  string         `json:"message"`
	Summary  []int          `json:"summary"`
	DayCount map[string]int `json:"daycount"`
	Messages []gosms.SMS    `json:"messages"`
}

// Cache templates
var templates = template.Must(template.ParseFiles("./templates/index.html"))

var authUsername string
var authPassword string

/* dashboard handlers */

// dashboard
func indexHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("--- indexHandler")
	// templates.ExecuteTemplate(w, "index.html", nil)
	// Use during development to avoid having to restart server
	// after every change in HTML
	t, _ := template.ParseFiles("./templates/index.html")
	t.Execute(w, nil)
}

// handle all static files based on specified path
// for now its /assets
func handleStatic(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	static := vars["path"]
	http.ServeFile(w, r, filepath.Join("./assets", static))
}

/* end dashboard handlers */

/* API handlers */

// push sms, allowed methods: POST
func sendSMSHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("--- sendSMSHandler")
	w.Header().Set("Content-type", "application/json")

	//TODO: validation
	r.ParseForm()
	mobile := r.FormValue("mobile")
	message := r.FormValue("message")
	uuid, _ := uuid.NewV1()

	mobile = numberToStandard(mobile)

	sendMessageToTg(mobile, message)

	sendMessageToWhatsUp(mobile, message)

	user, err := getUserOrMakeNew(mobile)
	if err != nil {
		log.Println(err)
		return
	}

	sms := &gosms.SMS{UUID: uuid.String(), Body: message, Retries: 0, User: user}
	gosms.EnqueueMessage(sms, true)

	smsresp := SMSResponse{Status: 200, Message: "ok"}
	var toWrite []byte
	toWrite, err = json.Marshal(smsresp)
	if err != nil {
		log.Println(err)
		//lets just depend on the server to raise 500
	}
	w.Write(toWrite)
}

// numberToStandard ???????????????? ???????????????????? ?????????? ?? ???????? +71112223344
func numberToStandard(phoneNumber string) string {

	if len(phoneNumber) < 3 {
		return phoneNumber
	}

	if phoneNumber[0] == '8' {
		phoneNumber = "7" + phoneNumber[1:]
	}

	if phoneNumber[0] != '+' {
		phoneNumber = "+" + phoneNumber
	}

	return phoneNumber
}

// getUserOrMakeNew ???????????????? ?????? ?????????????? ????????????????????????
func getUserOrMakeNew(phoneNumber string) (*gosms.User, error) {
	user, err := gosms.GetUserByPhoneNumber(phoneNumber)

	if err != nil {
		return nil, err
	}

	if user.ID != 0 {
		return user, nil
	}
	user = &gosms.User{
		PhoneNumber: phoneNumber,
	}
	user, err = gosms.InsertUser(user)
	if err != nil {
		return nil, err
	}

	return user, nil

}

func sendMessageToTg(phoneNumber string, message string) {
	users, err := gosms.GetUsersByPhoneNumber(phoneNumber)
	if err != nil {
		log.Printf("sendMessageToTg: %v", err)
		return
	}

	for _, user := range users {
		if user.ChatIdTelegram != "" {
			_, err = Bot.Send(NewUserTg(user.ChatIdTelegram), message)
			if err != nil {
				log.Printf("sendMessageToTg: %v", err)
				continue
			}
		}
	}
}

func sendMessageToWhatsUp(phoneNumber string, message string) {
	var (
		host             = "https://api.green-api.com"
		idInstance       = "9929"
		apiTokenInstance = "1af2ef5a2f4450904e02dff12f40dcdbb03e5e36380eac5fac"
	)

	url := fmt.Sprintf("%s/waInstance%s/sendMessage/%s", host, idInstance, apiTokenInstance)

	messageWhatsUp := &MessageWhatsUp{
		ChatId:  fmt.Sprintf("%s@c.us", phoneNumber[1:]),
		Message: message,
	}
	requestByte, err := json.Marshal(messageWhatsUp)
	log.Printf("+%v %s", requestByte, err)

	resp, err := http.Post(url, "application/json", bytes.NewReader(requestByte))

	log.Printf("%+v %s", resp, err)
}

// dumps JSON data, used by log view. Methods allowed: GET
func getLogsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("--- getLogsHandler")
	messages, _ := gosms.GetMessages("")
	summary, _ := gosms.GetStatusSummary()
	dayCount, _ := gosms.GetLast7DaysMessageCount()
	logs := SMSDataResponse{
		Status:   200,
		Message:  "ok",
		Summary:  summary,
		DayCount: dayCount,
		Messages: messages,
	}
	var toWrite []byte
	toWrite, err := json.Marshal(logs)
	if err != nil {
		log.Println(err)
		//lets just depend on the server to raise 500
	}
	w.Header().Set("Content-type", "application/json")
	w.Write(toWrite)
}

/* end API handlers */

func InitServer(host string, port string, username string, password string) error {
	log.Println("--- InitServer ", host, port)

	authUsername = username
	authPassword = password

	r := mux.NewRouter()
	r.StrictSlash(true)

	r.HandleFunc("/", use(indexHandler, basicAuth))

	// handle static files
	r.HandleFunc(`/assets/{path:[a-zA-Z0-9=\-\/\.\_]+}`, use(handleStatic, basicAuth))

	// all API handlers
	api := r.PathPrefix("/api").Subrouter()
	api.Methods("GET").Path("/logs/").HandlerFunc(use(getLogsHandler, basicAuth))
	api.Methods("POST").Path("/sms/").HandlerFunc(use(sendSMSHandler, basicAuth))

	http.Handle("/", r)

	bind := fmt.Sprintf("%s:%s", host, port)
	log.Println("listening on: ", bind)
	return http.ListenAndServe(bind, nil)

}

// See https://gist.github.com/elithrar/7600878#comment-955958 for how to extend it to suit simple http.Handler's
func use(h http.HandlerFunc, middleware ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for _, m := range middleware {
		h = m(h)
	}

	return h
}

func basicAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(authUsername) == 0 {
			h.ServeHTTP(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)

		s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(s) != 2 {
			http.Error(w, "Not authorized", 401)
			return
		}

		b, err := base64.StdEncoding.DecodeString(s[1])
		if err != nil {
			http.Error(w, err.Error(), 401)
			return
		}

		pair := strings.SplitN(string(b), ":", 2)
		if len(pair) != 2 || pair[0] != authUsername || pair[1] != authPassword {
			http.Error(w, "Not authorized", 401)
			return
		}

		h.ServeHTTP(w, r)
	}
}
