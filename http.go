package main

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"

	"go.uber.org/zap"
)

var updateTmpl = template.Must(template.New("update").Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Update OpenAI Instruction</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
        }
        form {
            max-width: 800px;
            margin: auto;
        }
        textarea {
            width: 100%;
            height: 800px;
        }
    </style>
    <script>
        async function handleFormSubmit(event) {
            event.preventDefault();
            const form = event.target;
            const formData = new FormData(form);
            const formDataObj = Object.fromEntries(formData.entries());

            try {
                const response = await fetch(form.action, {
                    method: form.method,
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(formDataObj)
                });

                const data = await response.text();
                alert(data);
                location.reload();
            } catch (error) {
                console.error('Error:', error);
                alert('Failed to submit form');
            }
        }
    </script>
</head>
<body>
    <form action="/config/openai_instruction" method="post" onsubmit="handleFormSubmit(event)">
        <label for="instruction">OpenAI Instruction:</label><br>
        <textarea id="instruction" name="instruction">{{.Instruction}}</textarea><br>
        <input type="submit" value="Submit">
    </form>
</body>
</html>
`))

func startHttpServer() {
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/config/openai_instruction", openAIInstructionHandler)

	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			logger.Fatal("Failed to start HTTP server", zap.Error(err))
		}
	}()

	logger.Info("Started HTTP server on :8080")
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Welcome to Murailo Bot"))
}

func openAIInstructionHandler(w http.ResponseWriter, r *http.Request) {
	appConfig, err := getAppConfig()
	if err != nil && err != sql.ErrNoRows {
		logger.Error("Failed to get app config", zap.Error(err))
		http.Error(w, "Failed to get app config", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case "GET":
		handleGetRequest(w, appConfig)
	case "POST":
		handlePostRequest(w, r, appConfig)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetRequest(w http.ResponseWriter, appConfig AppConfig) {
	err := updateTmpl.Execute(w, struct{ Instruction string }{Instruction: appConfig.OpenAIInstruction})
	if err != nil {
		logger.Error("Failed to render template", zap.Error(err))
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

func handlePostRequest(w http.ResponseWriter, r *http.Request, appConfig AppConfig) {
	var reqBody struct {
		Instruction string `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		logger.Error("Failed to parse JSON", zap.Error(err))
		http.Error(w, "Invalid JSON data", http.StatusBadRequest)
		return
	}
	instruction := reqBody.Instruction

	var err error
	if appConfig.ID == 0 {
		err = insertAppConfig(instruction)
	} else {
		err = updateAppConfig(instruction, appConfig.ID)
	}

	if err != nil {
		logger.Error("Failed to save configuration", zap.Error(err))
		http.Error(w, "Failed to save configuration", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("OpenAI Instruction updated successfully"))
}
