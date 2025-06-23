// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE

package miner

import (
	"fmt"
	"math/big"
	"net/smtp"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p"
)

// https://github.com/ethereum/go-ethereum/issues/21221#issuecomment-805852059
func weiToEther(wei *big.Int) *big.Float {
	f := new(big.Float)
	f.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	f.SetMode(big.ToNearestEven)
	if wei == nil {
		return f.SetInt64(0)
	}
	fWei := new(big.Float)
	fWei.SetPrec(236) //  IEEE 754 octuple-precision binary floating-point format: binary256
	fWei.SetMode(big.ToNearestEven)
	return f.Quo(fWei.SetInt(wei), big.NewFloat(params.Ether))
}

func fmtEth(wei *big.Int) string {
	f := weiToEther(wei)
	return fmt.Sprintf("%.9f", f)
}

func sendEmail(status bool, msg string, config EmailConfig, lg log.Logger) {
	lg.Info("Sending email notification...")
	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)

	emailSubject := "EthStorage Proof Submission: "
	if status {
		emailSubject += "✅ Success"
	} else {
		emailSubject += "❌ Failure"
	}

	emailBody := fmt.Sprintf("Subject: %s\r\n", emailSubject)
	emailBody += fmt.Sprintf("To: %s\r\n", config.To)
	emailBody += fmt.Sprintf("From: %s\r\n", config.From)

	localIP := p2p.GetLocalPublicIPv4()
	if localIP != nil {
		lg.Info("Using local IP for email sender", "ip", localIP.String())
		msg = "EthStorage Node Location: " + localIP.String() + "\r\n" + msg
	}
	emailBody += "\r\n" + msg

	err := smtp.SendMail(
		fmt.Sprintf("%s:%d", config.Host, config.Port),
		auth,
		config.From,
		config.To,
		[]byte(emailBody),
	)
	if err != nil {
		lg.Error("Failed to send email", "error", err, "config", config)
		fmt.Println(emailBody)
	} else {
		lg.Info("Email notification sent successfully!")
	}
}
