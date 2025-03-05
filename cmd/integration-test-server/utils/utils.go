package utils

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

func CheckKnowFailure(logFile string) (int, error) {
	return checkKnownFailure(logFile)
}

func checkKnownFailure(logFile string) (int, error) {
	count0, err := checkDiffNotMatchError(logFile)
	if err != nil {
		return 0, fmt.Errorf("checkDiffNotMatchError fail: err, %s", err.Error())
	}
	count1, err := checkInvalidSamplesError(logFile)
	if err != nil {
		return count0, fmt.Errorf("checkInvalidSamplesError fail: err, %s", err.Error())
	}
	count2, err := checkMinedTsTooSmallError(logFile)
	if err != nil {
		return count0, fmt.Errorf("checkMinedTsTooSmallError fail: err, %s", err.Error())
	}

	return count0 + count1 + count2, nil
}

func checkMinedTsTooSmallError(logFile string) (int, error) {
	// t=2025-02-21T03:31:56+0000 lvl=info msg="Submit mined result done"              shard=1 block=7,752,131 nonce=909,742 txSigner=0x8be2c9379eb69877F25aBa61a853eC4FCb0b273a hash=0xb4226d1baa9d92d1e289333a9c3a089632d3b460e5b63828de115d82fb63ca72
	// t=2025-02-21T03:32:00+0000 lvl=eror msg="Failed to submit mined result"         shard=1 block=7,752,130 error="failed to estimate gas: execution reverted: StorageContract: minedTs too small"

	file, err := os.OpenFile(logFile, os.O_RDONLY, 0755)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	fileScanner := bufio.NewScanner(file)
	fileScanner.Split(bufio.ScanLines)

	count := 0
	lastMinedBlocks := make(map[string]uint64)
	for fileScanner.Scan() {
		logText := fileScanner.Text()
		if strings.Contains(logText, "Submit mined result done") {
			shard, block, err := fetchShardAndBlock(logText)
			if err != nil {
				log.Error("fetchShardAndBlock error", "log", logText, "error", err.Error())
				continue
			}

			if b, ok := lastMinedBlocks[shard]; ok && b > block {
				continue
			}
			lastMinedBlocks[shard] = block
		} else if regexp.MustCompile(`Failed to submit mined result[\s\S]+minedTs too small`).MatchString(logText) {
			shard, block, err := fetchShardAndBlock(logText)
			if err != nil {
				log.Error("fetchShardAndBlock error", "log", logText, "error", err.Error())
				continue
			}
			if b, ok := lastMinedBlocks[shard]; ok && b > block {
				count++
			}
		}
	}

	return count, nil
}

func checkDiffNotMatchError(logFile string) (int, error) {
	// Description: "Mining info retrieved" -> "Failed to submit mined result"; shard and block equal which difficulty is not the same
	// Sample:
	// lvl=info msg="Mining info retrieved"                 shard=1 block=6,906,682 difficulty=13,006,115 lastMineTime=1,729,375,716 proofsSubmitted=2
	// lvl=eror msg="Failed to submit mined result"         shard=1 block=6,906,682 difficulty=1,729,376,532 error="failed to estimate gas: execution reverted: StorageContract: diff not match"

	file, err := os.OpenFile(logFile, os.O_RDONLY, 0755)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	fileScanner := bufio.NewScanner(file)
	fileScanner.Split(bufio.ScanLines)

	count := 0
	difficultyMap := make(map[string]string)
	for fileScanner.Scan() {
		logText := fileScanner.Text()
		if strings.Contains(logText, "Mining info retrieved") {
			block, diff, err := fetchBlockAndDifficulty(logText)
			if err != nil {
				log.Error("fetchBlockAndDifficulty error", "log", logText, "error", err.Error())
				continue
			}
			difficultyMap[block] = diff
		} else if regexp.MustCompile(`Failed to submit mined result[\s\S]+diff not match`).MatchString(logText) {
			block, diff, err := fetchBlockAndDifficulty(logText)
			if err != nil {
				log.Error("fetchBlockAndDifficulty error", "log", logText, "error", err.Error())
				continue
			}
			if originalDiff, ok := difficultyMap[block]; ok && strings.Compare(diff, originalDiff) != 0 {
				log.Warn("Known diff not match error", "block", block, "original difficulty", originalDiff, "latest difficulty", diff)
				count++
				continue
			}
		}
	}

	return count, nil
}

func checkInvalidSamplesError(logFile string) (int, error) {
	// description: 1. sample empty blob k, 2. download blob k, 3. submit mined result fail
	// Success Sample:
	// lvl=info msg="Get data hash"                         kvIndex=11742 hash=0x0000000000000000000000000000000000000000000000000000000000000000
	// lvl=info msg="Downloaded and encoded"                blockNumber=4,225,672 kvIdx=11742
	// lvl=info msg="Got storage proof"                     shard=1 block=6,906,682 kvIdx="[14613 11742]" sampleIdxsInKv="[1691 1859]"
	// lvl=info msg="Submit mined result done"              shard=1 block=6,906,682 nonce=739,669   txSigner=0x8be2c9379eb69877F25aBa61a853eC4FCb0b273a hash=0x4f212aa8f5b8867b6478f4cde9dc09609590ff4fc14997f7048ff32f2c96af0c

	// Invalid Sample 0:
	// lvl=info msg="Get data hash"                         kvIndex=11742 hash=0x0000000000000000000000000000000000000000000000000000000000000000
	// lvl=info msg="Downloaded and encoded"                blockNumber=4,225,672 kvIdx=11742
	// lvl=info msg="Got storage proof"                     shard=1 block=6,906,682 kvIdx="[14613 11742]" sampleIdxsInKv="[1691 1859]"
	// lvl=eror msg="Failed to submit mined result"         shard=1 block=6,906,682 error="failed to estimate gas: execution reverted: EthStorageContract2: invalid samples"

	// Invalid Sample 1:
	// lvl=info msg="Get data hash"                         kvIndex=11742 hash=0x0000000000000000000000000000000000000000000000000000000000000000
	// lvl=info msg="Got storage proof"                     shard=1 block=6,906,682 kvIdx="[14613 11742]" sampleIdxsInKv="[1691 1859]"
	// lvl=info msg="Downloaded and encoded"                blockNumber=4,225,672 kvIdx=11742
	// lvl=eror msg="Failed to submit mined result"         shard=1 block=6,906,682 error="failed to estimate gas: execution reverted: EthStorageContract2: invalid samples"

	// Invalid Sample 2:
	// t=2025-02-07T12:10:54+0000 lvl=info msg="Get data hash"                         kvIndex=11594 hash=0x0000000000000000000000000000000000000000000000000000000000000000
	// t=2025-02-07T12:11:34+0000 lvl=info msg="Got storage proof"                     shard=1 block=7,658,343 kvIdx="[11594 14173]" sampleIdxsInKv="[2817 2677]"
	// t=2025-02-07T12:11:34+0000 lvl=eror msg="Failed to submit mined result"         shard=1 block=7,658,343 error="failed to estimate gas: execution reverted: EthStorageContract2: invalid samples"
	// t=2025-02-07T12:11:35+0000 lvl=info msg="Downloaded and encoded"                blockNumber=2,036,896 kvIdx=11594

	file, err := os.OpenFile(logFile, os.O_RDONLY, 0755)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	fileScanner := bufio.NewScanner(file)
	fileScanner.Split(bufio.ScanLines)

	count := 0
	minedEmptyKVs := make(map[string]string)
	legacyKVs := make(map[string]string)
	suspiciousKVs := make(map[string]time.Time)
	for fileScanner.Scan() {
		logText := fileScanner.Text()
		if regexp.MustCompile(`Get data hash[\s\S]+hash=0x0000000000000000000000000000000000000000000000000000000000000000`).MatchString(logText) {
			kvIdx := extractWithName(logText, `kvIndex=(?P<kvIdx>[\d]+)`, "kvIdx")
			if kvIdx != "" {
				minedEmptyKVs[kvIdx] = ""
			}
		} else if strings.Contains(logText, `Downloaded and encoded`) {
			kvIdx := extractWithName(logText, `kvIdx=(?P<kvIdx>[\d]+)`, "kvIdx")
			if kvIdx == "" {
				continue
			}
			block, ok := minedEmptyKVs[kvIdx]
			if !ok {
				continue
			}
			if ft, e := suspiciousKVs[kvIdx]; e {
				t, got := fetchLogTime(logText)
				if got && t.Sub(ft).Seconds() < 2 {
					count++
					delete(minedEmptyKVs, kvIdx)
					delete(suspiciousKVs, kvIdx)
					continue
				}
			}
			legacyKVs[kvIdx] = block
		} else if strings.Contains(logText, `Got storage proof`) {
			block, kvIdxes, err := fetchMinedBlockAndKVIdx(logText)
			if err != nil {
				log.Error("checkInvalidSamplesError error", "log", logText, "error", err.Error())
			}
			for kvIdx := range minedEmptyKVs {
				if !strings.Contains(kvIdxes, kvIdx) {
					continue
				}
				minedEmptyKVs[kvIdx] = block
				if _, ok := legacyKVs[kvIdx]; ok {
					legacyKVs[kvIdx] = block
				}
			}
		} else if regexp.MustCompile(`Failed to submit mined result[\s\S]+invalid samples`).MatchString(logText) {
			for kvIdx, block := range legacyKVs {
				if block != "" && strings.Contains(logText, block) {
					log.Warn("Known error", "block", block, "kvIdx", kvIdx, "error", "invalid samples")
					delete(legacyKVs, kvIdx)
					count++
					continue
				}
				delete(minedEmptyKVs, kvIdx)
			}
			for kvIdx, block := range minedEmptyKVs {
				if block != "" && strings.Contains(logText, block) {
					if t, ok := fetchLogTime(logText); ok {
						suspiciousKVs[kvIdx] = t
					}
				}
			}
		} else if strings.Contains(logText, "Submit mined result done") {
			for kvIdx, block := range minedEmptyKVs {
				if block != "" && strings.Contains(logText, block) {
					delete(minedEmptyKVs, kvIdx)
				}
				continue
			}
		}
	}

	return count, nil
}

func fetchShardAndBlock(text string) (shard string, block uint64, err error) {
	patten := `shard=(?P<shard>[\d])[\s]+block=(?P<block>[\d{1,3}(,\d{3})*]+)[\s]+`
	shard, block, err = "", uint64(0), nil

	results := extract(text, patten)
	if len(results) == 2 {
		for name, value := range results {
			if strings.Compare(name, "shard") == 0 {
				shard = value
			} else if strings.Compare(name, "block") == 0 {
				block, err = strconv.ParseUint(strings.Replace(value, ",", "", 5), 10, 64)
				if err != nil {
					return "", 0, fmt.Errorf("convert string to block fail, string %s; error: %s", value, err.Error())
				}
			}
		}
	} else {
		err = fmt.Errorf("extract shard and block fail, patten: %s, results %v， error: wrong log format", patten, results)
	}

	return
}

func fetchLogTime(text string) (time.Time, bool) {
	timeStr := extractWithName(text, `t=(?P<timeStr>[\s\S]+)\+0000`, "timeStr")
	t, err := time.Parse("2006-01-02T15:04:05", timeStr)
	if err != nil {
		log.Warn("Parse time string fail", "timeStr", timeStr)
		return time.Now(), false
	}
	return t, true
}

func fetchMinedBlockAndKVIdx(text string) (block string, kvIdx string, err error) {
	patten := `block=(?P<block>[\d{1,3}(,\d{3})*]+)[\s]+kvIdx=\"\[(?P<kvIdx>\d+ \d+)\]\"`

	results := extract(text, patten)
	if len(results) == 2 {
		for name, value := range results {
			if strings.Compare(name, "block") == 0 {
				block = value
			} else if strings.Compare(name, "kvIdx") == 0 {
				kvIdx = value
			}
		}
	}

	if block == "" || kvIdx == "" {
		err = fmt.Errorf("extract mined block and nonce fail, patten: %s, error: wrong log format", patten)
	}

	return
}

func fetchBlockAndDifficulty(text string) (string, string, error) {
	patten := `shard=(?P<shard>[\d])[\s]+block=(?P<block>[\d{1,3}(,\d{3})*]+)[\s]+difficulty=(?P<difficulty>[\d{1,3}(,\d{3})*]+)`
	shard, block, diff := "", "", ""

	results := extract(text, patten)
	if len(results) == 3 {
		for name, value := range results {
			if strings.Compare(name, "shard") == 0 {
				shard = value
			} else if strings.Compare(name, "block") == 0 {
				block = value
			} else if strings.Compare(name, "difficulty") == 0 {
				diff = value
			}
		}
	}

	if shard == "" || block == "" || diff == "" {
		return "", "", fmt.Errorf("extract block and difficulty fail, patten: %s， error: wrong log format", patten)
	}

	return fmt.Sprintf("%s-%s", shard, block), diff, nil
}

func extractWithName(text, patten, name string) string {
	results := extract(text, patten)
	return results[name]
}

func extract(text, patten string) map[string]string {
	results := make(map[string]string)
	re := regexp.MustCompile(patten)

	matches := re.FindStringSubmatch(text)
	if matches != nil {
		for i, name := range re.SubexpNames() {
			if i != 0 && name != "" {
				results[name] = matches[i]
			}
		}
	}

	return results
}
