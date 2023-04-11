package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/go-co-op/gocron"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The configuration should be externalized
const (
	FileName                   = "promotions.csv"
	HalfHour                   = 30 * 60 * 1000 * 1000 * 1000
	RedisPoolSize              = 100
	BatchSize                  = 100
	RedisHost                  = "localhost"
	RedisPort                  = "6379"
	RedisPassword              = ""
	RedisDB                    = 0
	ImportJobIntervalInMinutes = 30
)

var mutex sync.RWMutex

var syncInProgress = false

var redisClient *redis.Client

// Promotion If we need to manipulate data right types should be used e.g. Money for price
type Promotion struct {
	Id             string  `json:"id"`
	Price          float64 `json:"price"`
	ExpirationDate string  `json:"expiration_date"`
}

func scheduleDataImport() {
	s := gocron.NewScheduler(time.UTC)
	s.Every(ImportJobIntervalInMinutes).Minutes().Do(func() {
		log.Debug().Msg("Synchronizing database with the filesystem")
		mutex.Lock()
		redisClient.FlushDB(redisClient.Context())
		log.Debug().Msg("Database is flushed.")
		syncInProgress = true
		mutex.Unlock()
		file := openFile()
		if file != nil {
			defer file.Close()
			lines, channels := createChannels()
			scanner := bufio.NewScanner(file)
			go func() {
				defer close(lines)
				for scanner.Scan() {
					lines <- scanner.Text()
				}
				if err := scanner.Err(); err != nil {
					log.Warn().Str("filename", FileName).Err(err).Msg("File reading has been interrupted.")
				}
			}()
			waitForWorkers(channels...)
			log.Info().Msg("Database has been synchronized with the filesystem")
		}
		syncInProgress = false
	})
	s.StartBlocking()
}

func openFile() *os.File {
	file, err := os.Open(FileName)
	if err != nil {
		log.Error().Str("filename", FileName).Err(err).Msg("Can't open the file.")
	}
	return file
}

func createChannels() (chan string, []<-chan string) {
	lines := make(chan string)
	var channels []<-chan string
	for i := 0; i < BatchSize; i++ {
		channels = append(channels, storePromotions(lines))
	}
	return lines, channels
}

func storePromotions(lines <-chan string) <-chan string {
	finished := make(chan string)
	go func() {
		defer close(finished)
		for line := range lines {
			promotion := parsePromotion(line)
			serializedPromotion, err := json.Marshal(promotion)
			if err != nil {
				log.Warn().Str("promotion", line).Err(err).Msg("Can't serialize promotion.")

			} else {
				err := redisClient.Set(redisClient.Context(), promotion.Id, serializedPromotion, -1).Err()
				if err != nil {
					log.Error().Err(err).Msg("Can't communicate with Redis.")
				}
			}
			finished <- line
		}
	}()
	return finished
}

func parsePromotion(line string) Promotion {
	split := strings.Split(line, ",")
	float, _ := strconv.ParseFloat(split[1], 64)
	formattedPrice := math.Ceil(float*100) / 100
	date, _ := time.Parse("2006-01-02 15:04:05 +0200", split[2][:strings.LastIndex(split[2], " ")])
	promo := Promotion{Id: split[0], Price: formattedPrice, ExpirationDate: date.Format("2006-01-02 15:04:05")}
	return promo
}

func waitForWorkers(cs ...<-chan string) {
	var wg sync.WaitGroup
	out := make(chan string)
	output := func(c <-chan string) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	for range out {
	}
}

func retrievePromotion(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["id"]
	mutex.RLock()
	for syncInProgress {
		//TODO handle timeout
		_, err := redisClient.Get(redisClient.Context(), key).Result()
		if err != nil {
			continue
		}
		break
	}
	mutex.RUnlock()
	get, err := redisClient.Get(redisClient.Context(), key).Result()
	if err != nil {
		log.Info().Str("id", key).Msg("Can't find the promotion.")
		w.WriteHeader(404)
		fmt.Fprintf(w, "Promotion is not found!!!")
	} else {
		fmt.Fprintf(w, get)
	}
}

func handleRequests() {
	myRouter := mux.NewRouter().StrictSlash(true)
	myRouter.HandleFunc("/promotions/{id}", retrievePromotion)
	log.Fatal().Err(http.ListenAndServe(":"+getEnv("PORT", "1321"), myRouter)).Msg("Failed to start http server.")
}

func connectToRedis() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_HOST", RedisHost) + ":" + getEnv("REDIS_PORT", RedisPort),
		Password: getEnv("REDIS_PASSWORD", RedisPassword),
		DB:       RedisDB,
		PoolSize: RedisPoolSize,
	})
	return client
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func main() {
	redisClient = connectToRedis()
	defer func(client *redis.Client) {
		err := client.Close()
		if err != nil {
			log.Error().Err(err).Msg("Failed to close Redis connection.")
		}
	}(redisClient)
	go handleRequests()
	scheduleDataImport()
}
