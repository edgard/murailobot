package main

import (
	"database/sql"
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
            width: 80%;
            margin: auto;
        }
        textarea {
            width: 100%;
            height: 800px;
        }
    </style>
</head>
<body>
    <form action="/config/openai_instruction/update" method="post">
        <label for="instruction">OpenAI Instruction:</label><br>
        <textarea id="instruction" name="instruction">{{.Instruction}}</textarea><br>
        <input type="submit" value="Submit">
    </form>
</body>
</html>
`))

func startHttpServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to Murailo Bot"))
	})
	http.HandleFunc("/config/openai_instruction", serveUpdateForm)
	http.HandleFunc("/config/openai_instruction/update", updateOpenAIInstruction)

	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			logger.Fatal("Failed to start HTTP server", zap.Error(err))
		}
	}()

	logger.Info("Started HTTP server")
}

func serveUpdateForm(w http.ResponseWriter, r *http.Request) {
	appConfig, err := getAppConfig()
	if err != nil && err != sql.ErrNoRows {
		logger.Error("Failed to get app config", zap.Error(err))
		http.Error(w, "Failed to get app config", http.StatusInternalServerError)
		return
	}
	if err := updateTmpl.Execute(w, struct{ Instruction string }{Instruction: appConfig.OpenAIInstruction}); err != nil {
		logger.Error("Failed to render template", zap.Error(err))
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

func updateOpenAIInstruction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	instruction := r.FormValue("instruction")

	appConfig, err := getAppConfig()
	if err != nil && err != sql.ErrNoRows {
		logger.Error("Failed to get app config", zap.Error(err))
		http.Error(w, "Failed to get app config", http.StatusInternalServerError)
		return
	}

	if appConfig.ID == 0 {
		err = insertAppConfig(instruction)
		if err != nil {
			logger.Error("Failed to create configuration", zap.Error(err))
			http.Error(w, "Failed to create configuration", http.StatusInternalServerError)
			return
		}
	} else {
		err = updateAppConfig(instruction, appConfig.ID)
		if err != nil {
			logger.Error("Failed to update configuration", zap.Error(err))
			http.Error(w, "Failed to update configuration", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OpenAI Instruction updated successfully"))
}
