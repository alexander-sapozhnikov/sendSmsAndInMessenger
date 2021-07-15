package main

func initWhatsBot() {

}

// MessageWhatsUp структура для отправки сообщений whats up
type MessageWhatsUp struct {
	ChatId  string `json:"chatId"`
	Message string `json:"message"`
}
