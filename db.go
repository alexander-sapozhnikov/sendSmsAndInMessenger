package gosms

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"os"
)

var db *sql.DB

func InitDB(driver, dbname string) (*sql.DB, error) {
	var err error
	createDb := false
	if _, err := os.Stat(dbname); os.IsNotExist(err) {
		log.Printf("InitDB: database does not exist %s", dbname)
		createDb = true
	}
	db, err = sql.Open(driver, dbname)
	if createDb {
		if err = syncDB(); err != nil {
			return nil, errors.New("Error creating database")
		}
	}
	return db, nil
}

func syncDB() error {
	log.Println("--- syncDB")
	//create messages table
	createMessages := `-- таблица пользователей
CREATE TABLE IF NOT EXISTS usr (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    phone_number char(15)  not null,
    chat_id_telegram integer
);

-- таблица сообщений
CREATE TABLE messages (
      id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
      fk_usr integer NOT NULL,
      uuid char(32) UNIQUE NOT NULL,
      message char(160)   NOT NULL,
      status  INTEGER DEFAULT 0,
      retries INTEGER DEFAULT 0,
      device string NULL,
      created_at TIMESTAMP default CURRENT_TIMESTAMP,
      updated_at TIMESTAMP,
      FOREIGN KEY (fk_usr) REFERENCES usr(id)
);`
	_, err := db.Exec(createMessages, nil)
	fmt.Println("err", err)
	return err
}

func insertMessage(sms *SMS) error {
	log.Println("--- insertMessage ", sms)
	tx, err := db.Begin()
	if err != nil {
		log.Println("insertMessage: ", err)
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO messages(uuid, message, fk_usr) VALUES(?, ?, ?)")
	if err != nil {
		log.Println("insertMessage: ", err)
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(sms.UUID, sms.Body, sms.User.ID)
	if err != nil {
		log.Println("insertMessage: ", err)
		return err
	}
	tx.Commit()
	return nil
}

func InsertUser(user *User) (*User, error) {
	log.Println("--- insertUser ", user)
	tx, err := db.Begin()
	if err != nil {
		log.Println("insertUser: ", err)
		return nil, err
	}
	stmt, err := tx.Prepare("INSERT INTO usr(phone_number, chat_id_telegram) VALUES(?, ?)")
	if err != nil {
		log.Println("insertUser: ", err)
		return nil, err
	}
	defer stmt.Close()
	res, err := stmt.Exec(user.PhoneNumber, user.ChatIdTelegram)
	if err != nil {
		log.Println("insertUser: ", err)
		return nil, err
	}

	user.ID, err = res.LastInsertId()
	if err != nil {
		log.Println("insertUser: ", err)
		return nil, err
	}


	tx.Commit()
	return user, nil
}
func UpdateUser(user *User) error {
	log.Println("--- updateUser ", user)
	tx, err := db.Begin()
	if err != nil {
		log.Println("updateUser: ", err)
		return err
	}
	stmt, err := tx.Prepare("UPDATE usr SET phone_number = ?, chat_id_telegram = ? WHERE id = ?")
	if err != nil {
		log.Println("updateUser: ", err)
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(user.PhoneNumber, user.ChatIdTelegram, user.ID)
	if err != nil {
		log.Println("updateUser: ", err)
		return err
	}

	tx.Commit()
	return nil
}


// GetUserByPhoneNumber получение одного из пользователей с номером
func GetUserByPhoneNumber(phoneNumber string) (*User, error) {
	log.Println("--- getUserByPhoneNumber ")

	query := "SELECT id, phone_number, chat_id_telegram FROM usr WHERE phone_number = ? LIMIT 1"
	log.Println("getUserByPhoneNumber: ", query)

	row := db.QueryRow(query, phoneNumber)
	err := row.Err()
	if err != nil {
		log.Println("getUserByPhoneNumber: ", err)
		return nil, err
	}

	user := &User{}
	row.Scan(&user.ID, &user.PhoneNumber, &user.ChatIdTelegram)

	return user, nil
}

// GetUserByChatIdTg получение пользователя по номеру чата в телеграм
func GetUserByChatIdTg(chatID string) (*User, error) {
	log.Println("--- GetUserByChatIdTg ")

	query := "SELECT id, phone_number, chat_id_telegram FROM usr WHERE chat_id_telegram = ? LIMIT 1"
	log.Println("GetUserByChatIdTg: ", query)

	row := db.QueryRow(query, chatID)
	err := row.Err()
	if err != nil {
		log.Println("GetUserByChatIdTg: ", err)
		return nil, err
	}

	user := &User{}
	row.Scan(&user.ID, &user.PhoneNumber, &user.ChatIdTelegram)

	return user, nil
}

// GetUsersByPhoneNumber получение всех пользователей на определенный номер
func GetUsersByPhoneNumber(phoneNumber string) ([]*User, error) {
	log.Println("--- getUsersByPhoneNumber ")

	query := "SELECT id, phone_number, chat_id_telegram FROM usr WHERE phone_number = ?"
	log.Println("getUsersByPhoneNumber: ", query)

	rows, err := db.Query(query, phoneNumber)
	defer rows.Close()

	if err != nil {
		log.Println("getUsersByPhoneNumber: ", err)
		return nil, err
	}

	var users []*User

	for rows.Next() {
		user := &User{}
		err = rows.Scan(&user.ID, &user.PhoneNumber, &user.ChatIdTelegram)

		if err != nil{
			log.Println("getUsersByPhoneNumber: ", err)
		}

		users = append(users, user)
	}

	return users, nil
}

func updateMessageStatus(sms SMS) error {
	log.Println("--- updateMessageStatus ", sms)
	tx, err := db.Begin()
	if err != nil {
		log.Println("updateMessageStatus: ", err)
		return err
	}
	stmt, err := tx.Prepare("UPDATE messages SET status=?, retries=?, device=?, updated_at=DATETIME('now') WHERE uuid=?")
	if err != nil {
		log.Println("updateMessageStatus: ", err)
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(sms.Status, sms.Retries, sms.Device, sms.UUID)
	if err != nil {
		log.Println("updateMessageStatus: ", err)
		return err
	}
	tx.Commit()
	return nil
}

func getPendingMessages(bufferSize int) ([]SMS, error) {
	log.Println("--- getPendingMessages ")
	query := fmt.Sprintf("SELECT uuid, message, status, retries, phone_number " +
		" FROM messages LEFT JOIN usr  ON usr.id = messages.fk_usr " +
		" WHERE status!=%v AND retries<%v LIMIT %v",
		SMSProcessed, SMSRetryLimit, bufferSize)
	log.Println("getPendingMessages: ", query)

	rows, err := db.Query(query)
	if err != nil {
		log.Println("getPendingMessages: ", err)
		return nil, err
	}
	defer rows.Close()

	var messages []SMS

	for rows.Next() {
		sms := SMS{
			User: &User{},
		}
		rows.Scan(&sms.UUID, &sms.Body, &sms.Status, &sms.Retries, &sms.User.PhoneNumber)
		messages = append(messages, sms)
	}
	rows.Close()
	return messages, nil
}

func GetMessages(filter string) ([]SMS, error) {
	/*
	   expecting filter as empty string or WHERE clauses,
	   simply append it to the query to get desired set out of database
	*/
	log.Println("--- GetMessages")
	query := fmt.Sprintf("SELECT uuid, message, status, retries, phone_number, device, created_at, updated_at " +
		" FROM messages LEFT JOIN usr ON usr.id = messages.fk_usr %v", filter)
	log.Println("GetMessages: ", query)

	rows, err := db.Query(query)
	if err != nil {
		log.Println("GetMessages: ", err)
		return nil, err
	}
	defer rows.Close()

	var messages []SMS

	for rows.Next() {
		sms := SMS{
			User: &User{},
		}
		rows.Scan(&sms.UUID, &sms.Body, &sms.Status, &sms.Retries, &sms.User.PhoneNumber, &sms.Device, &sms.CreatedAt, &sms.UpdatedAt)
		messages = append(messages, sms)
	}
	rows.Close()
	return messages, nil
}

func GetLast7DaysMessageCount() (map[string]int, error) {
	log.Println("--- GetLast7DaysMessageCount")

	rows, err := db.Query(`SELECT strftime('%Y-%m-%d', created_at) as datestamp,
    COUNT(id) as messagecount FROM messages GROUP BY datestamp
    ORDER BY datestamp DESC LIMIT 7`)
	if err != nil {
		log.Println("GetLast7DaysMessageCount: ", err)
		return nil, err
	}
	defer rows.Close()

	dayCount := make(map[string]int)
	var day string
	var count int
	for rows.Next() {
		rows.Scan(&day, &count)
		dayCount[day] = count
	}
	rows.Close()
	return dayCount, nil
}

func GetStatusSummary() ([]int, error) {
	log.Println("--- GetStatusSummary")

	rows, err := db.Query(`SELECT status, COUNT(id) as messagecount 
    FROM messages GROUP BY status ORDER BY status`)
	if err != nil {
		log.Println("GetStatusSummary: ", err)
		return nil, err
	}
	defer rows.Close()

	var status, count int
	statusSummary := make([]int, 3)
	for rows.Next() {
		rows.Scan(&status, &count)
		statusSummary[status] = count
	}
	rows.Close()
	return statusSummary, nil
}
