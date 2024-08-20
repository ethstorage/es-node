// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package integration

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

var (
	letters            = []rune("abcdefghijklmnopqrstuvwxyz ")
	maxWordsInSentence = 20
	maxSentenceLength  = 20
	rdn                = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func generateWord(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letters[rdn.Intn(len(letters))]
	}
	return string(b)
}

func generateRandomSentence() string {
	numWords := rdn.Intn(maxWordsInSentence) + 1
	words := make([]string, numWords)
	for i := 0; i < numWords; i++ {
		wordLength := rdn.Intn(maxSentenceLength) + 1
		words[i] = generateWord(wordLength)
	}
	return fmt.Sprintf("%s.", strings.Join(words, " "))
}

func generateRandomContent(sizeInKB int) []byte {
	content := ""
	for len(content) < sizeInKB*1024 {
		sentence := generateRandomSentence()
		if len(content+sentence) < sizeInKB*1024 {
			content += sentence + " "
		} else {
			break
		}
	}
	return []byte(content)
}
func generateRandomBlobs(blobLen int) []byte {
	return generateRandomContent(128 * 31 / 32 * blobLen)
}
