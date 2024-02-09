package ToastJam

import (
	"bytes"
	"dev/mattbachmann/chatbotcli/internal/bots"
	"dev/mattbachmann/chatbotcli/internal/integrations"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type ToastJamResponse struct {
	Timestamp     string    `json:"timestamp"`
	CorrelationId string    `json:"correlation_id"`
	ToastTeam     string    `json:"toast_team"`
	ToastProduct  string    `json:"toast_product"`
	Model         string    `json:"model"`
	Messages      []Message `json:"messages"`
	RestaurantId  string    `json:"restaurant_id"`
}

type Message struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

type Parameters struct {
	AiUserType    string   `json:"ai_user_type"`
	DoSample      bool     `json:"do_sample"`
	MaxNewTokens  int      `json:"max_new_tokens"`
	StopSequences []string `json:"stop_sequences"`
	Temperature   float64  `json:"temperature"`
	TopK          int      `json:"top_k"`
	TopP          float64  `json:"top_p"`
}

type Request struct {
	CorrelationId       string     `json:"correlation_id"`
	LogResults          bool       `json:"log_results"`
	Messages            []Message  `json:"messages"`
	Model               string     `json:"model"`
	Parameters          Parameters `json:"parameters"`
	RequestType         string     `json:"request_type"`
	Timestamp           string     `json:"timestamp"`
	ToastJamRequestType string     `json:"toast_jam_request_type"`
	ToastProduct        string     `json:"toast_product"`
	ToastTeam           string     `json:"toast_team"`
	UseGuardrail        bool       `json:"use_guardrail"`
}

type ToastJam struct {
	Name string
}

func getRemainingTokenTime(toastToken string) (int64, error) {
	if toastToken == "" {
		return 0, errors.New("no token")
	}
	parts := strings.Split(toastToken, ".")
	if len(parts) != 3 {
		return 0, errors.New("auth token invalid")
	}
	toastInfoRaw, err := b64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errors.New("auth token invalid")
	}

	var toastInfo map[string]interface{}
	if err := json.Unmarshal(toastInfoRaw, &toastInfo); err != nil {
		return 0, errors.New("auth token invalid")
	}
	remainingTime := int64(toastInfo["exp"].(float64)) - time.Now().Unix()
	if remainingTime < 0 {
		return 0, errors.New("auth token expired")
	}
	return remainingTime, nil
}

func formatRemainingTime(remainingTokenTime int64) string {
	minutes := int(remainingTokenTime / 60)
	seconds := remainingTokenTime % 60
	return fmt.Sprintf("%d minutes and %d seconds", minutes, seconds)
}

func (b ToastJam) GetBotResponse(userLines []string, botLines []bots.BotResponse, systemPrompt string) bots.BotResponse {
	toastToken := os.Getenv("TOAST_AUTH_TOKEN")
	remainingTokenTime, err := getRemainingTokenTime(toastToken)
	formattedRemainingTime := formatRemainingTime(remainingTokenTime)
	if err != nil {
		return bots.BotResponse{
			Content:  err.Error(),
			Metadata: map[string]string{},
		}
	}
	toastResponse := makeCall(
		userLines,
		botLines,
		systemPrompt,
		0,
		toastToken,
	)
	return bots.BotResponse{
		Content: toastResponse.Messages[len(toastResponse.Messages)-1].Content,
		Metadata: map[string]string{
			"auth_expires_in": formattedRemainingTime,
		},
	}
}

func makeCall(
	userLines []string,
	botLines []bots.BotResponse,
	systemPrompt string,
	messagesToCut int,
	toastToken string,
) ToastJamResponse {
	client := &http.Client{}

	request := Request{
		CorrelationId: uuid.New().String(),
		LogResults:    false,
		Messages:      ConstructMessages(userLines, botLines, systemPrompt, messagesToCut),
		Model:         "llama_2_13b_chat",
		Parameters: Parameters{
			AiUserType:    "llm",
			DoSample:      false,
			MaxNewTokens:  1024,
			StopSequences: []string{},
			Temperature:   0.2,
			TopK:          50,
			TopP:          .5,
		},
		RequestType:         "COMPLETION",
		Timestamp:           time.Now().Format("2006-01-02T15:04:05.000Z"),
		ToastJamRequestType: "TOAST_JAM",
		ToastProduct:        "chatbot-cli",
		ToastTeam:           "bachmann",
		UseGuardrail:        true,
	}

	postBody, err := json.Marshal(request)
	if err != nil {
		panic(err)
	}
	requestBody := bytes.NewBuffer(postBody)
	req, err := http.NewRequest(
		"POST",
		"https://preprod.eng.toasttab.com/api/service/ds-model/v1/scone/ds-toast-jam/",
		requestBody,
	)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", toastToken))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Toast-Restaurant-External-Id", "c7b9367b-bc36-49ca-99e6-b4417b335476")
	req.Header.Add("Toast-Management-Set-Guid", "50f552fc-cc24-4cbe-84e1-20d96e644c8b")
	req.Header.Add("Toast-Restaurant-Set-Guid", "a60f8d88-2ce1-4d70-a478-c42d493bf286")

	resp, err := integrations.Retry(3, 1, client.Do, req)
	if err != nil {
		panic(err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var toastJamResponse ToastJamResponse
	err = json.Unmarshal(body, &toastJamResponse)
	if err != nil {
		panic(err)
	}
	return toastJamResponse
}

func ConstructMessages(userLines []string, botLines []bots.BotResponse, systemPrompt string, messagesToCut int) []Message {
	var messages []Message
	messages = append(messages, Message{systemPrompt, "SYSTEM"})
	for i := messagesToCut; i < len(userLines); i++ {
		messages = append(messages, Message{userLines[i], "USER"})
		if i < len(botLines) {
			messages = append(messages, Message{botLines[i].Content, "LLM"})
		}
	}
	return messages

}
