package main

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type tokenCacheTime struct {
	validUntil time.Time
}

var (
	tokenCache = make(map[string]tokenCacheTime)
	cacheMutex sync.Mutex
	cacheTime  = 30 * time.Minute
)

func noCacheHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		token := extractTokenFromRequest(r)
		if token == "" || !isValidToken(token) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			log.Println("Неправильный токен: ")
			return
		}
		log.Println("Успешно ответили на запрос")
		h.ServeHTTP(w, r)
	})
}

func extractTokenFromRequest(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

func isValidToken(token string) bool {
	cacheMutex.Lock()
	item, found := tokenCache[token]
	cacheMutex.Unlock()

	if found && time.Now().Before(item.validUntil) {
		return true
	}

	if verifyTokenExternally(token) {
		cacheMutex.Lock()
		tokenCache[token] = tokenCacheTime{
			validUntil: time.Now().Add(cacheTime),
		}
		cacheMutex.Unlock()
		return true
	}
	return false
}

func verifyTokenExternally(token string) bool {
	req, err := http.NewRequest("GET", "http://37.186.117.22:25565/launcher/Protected/", nil)
	if err != nil {
		log.Println("Ошибка создания запроса: ", err)
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("Ошибка при обращении к auth серверу: ", err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func startCacheCleanup() {
	ticker := time.NewTicker(1 * time.Minute)

	go func() {
		for range ticker.C {
			now := time.Now()
			cacheMutex.Lock()
			for token, item := range tokenCache {
				if now.After(item.validUntil) {
					delete(tokenCache, token)
					log.Println("Удалили токен по таймеру")
				}
			}
			cacheMutex.Unlock()
		}
	}()
}

func main() {
	fs := http.FileServer(http.Dir("./public"))

	handler := noCacheHandler(fs)

	http.Handle("/", handler)

	startCacheCleanup()

	log.Println("Listening on :9090")
	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Fatal(err)
	}
}
