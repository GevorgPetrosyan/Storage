# The Storage Problem

## Overview
The following repo contains solution of interview challenge.

## Installation

### Docker

1. Install Docker
2. Run ```docker-compose up``` command(change redis image if you're not running on Apple M1)
3. Example to access http://localhost:1321/promotions/898599c9-b7cd-40da-a198-6f59c6b274cd data

### Standard Deployment
1. Install Golang 1.20 and Redis server
2. Configure Redis address and credentials in the app.go file
    ```go 
    const (
        RedisHost                  = "localhost"
        RedisPort                  = "6379"
        RedisPassword              = ""
        RedisDB                    = 0
    )
    ```
3. Run  ```go mod download``` command then run ```go run app.go ``` command to start the application


## Solution

I read the file into channels and then concurrently sent data to Redis. I've used a boolean flag(synchronizationInProgress) which is updated inside Write Lock when I clean old data from Redis. Also, I used read lock to not retrieve inconsistent data, but when data is already cleared and the item which is user accessing is already stored then the user will get it without waiting until the end of synchronization. When we will have huge data during synchronization if the user wants to access data which is at the end of the file he or she can wait a while or request can timeout. With the current data size on my machine, the synchronization takes a few seconds. If we need to handle huge data we can cluster database(it can be Redis or any distributed data store, we should benchmark several options based on requirements to choose the right one). When we scale the application the lock should be externalized into the database and we can deploy one instance to write data and based on load several instances for reading. Also, I had a solution in mind to read the file via chunks(like RandomAccessFile in Java) using goroutines. With this approach, I could distribute even the writing part of the application. Again, there are several options and every solution has its benefits and drawbacks. We can only compromise the solution which fits our requirements.