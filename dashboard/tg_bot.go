package main

import (
	tb "go_modules/src/gopkg.in/tucnak/telebot.v2"
	"gosms"
	"log"
	"regexp"
	"strconv"
	"time"
)

var Bot *tb.Bot

func initTgBot() {
	var err error
	Bot, err = tb.NewBot(tb.Settings{
		Token:  "1885190357:AAGLAxRByT9IAOuLELFIV90iefPzpCC0N4Q",
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})

	if err != nil {
		log.Fatal(err)
		return
	}

	Bot.SetCommands([]tb.Command{{
		Text:        "/start",
		Description: "Запуск бота.",
	}})

	Bot.Handle("/start", func(m *tb.Message) {
		Bot.Send(m.Chat, "Пришлите номер телефона для привязки")
	})

	Bot.Handle(tb.OnText, getPhoneNumberFromUser)

	go Bot.Start()
}

var r, _ = regexp.Compile(`\+?[78]\d{10}`)

// getPhoneNumberFromUser получаем номер для привязки к аккаунту телеграм
func getPhoneNumberFromUser(m *tb.Message) {
	var (
		number   = ""
		isNumber = false
	)

	for _, entity := range m.Entities {
		if entity.Type == tb.EntityURL {
			isNumber = true
			number = m.Text[entity.Offset : entity.Offset+entity.Length]
			break
		}
	}

	if !isNumber && r.MatchString(m.Text) {
		isNumber = true
		number = r.FindString(m.Text)
	}

	if !isNumber {
		Bot.Send(m.Chat, "Пришлите номер телефона для привязки")
		return
	}

	number = numberToStandard(number)
	user, err := gosms.GetUserByChatIdTg(number)
	if err != nil {
		log.Printf("Error get user %v", err)
		Bot.Send(m.Chat, "Произошла ошибка при добавление номера.")
		return
	}

	user.PhoneNumber = number
	user.ChatIdTelegram = strconv.Itoa(int(m.Chat.ID))

	if user.ID == 0 {
		_, err = gosms.InsertUser(user)
	} else {
		err = gosms.UpdateUser(user)
	}

	if err != nil {
		log.Printf("Error update user %v", err)
		Bot.Send(m.Chat, "Произошла ошибка при сохранение номера.")
		return
	}
	Bot.Send(m.Chat, "Номер успешно привязан.")

}

// UserTg структура для отправки сообщений
type UserTg struct {
	chatId string
}

func NewUserTg(chatId string) *UserTg {
	return &UserTg{chatId: chatId}
}

func (u *UserTg) Recipient() string {
	return u.chatId
}
