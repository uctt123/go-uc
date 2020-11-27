
















package utils

import (
	"fmt"

	"github.com/ethereum/go-ethereum/console/prompt"
)




func GetPassPhrase(text string, confirmation bool) string {
	if text != "" {
		fmt.Println(text)
	}
	password, err := prompt.Stdin.PromptPassword("Password: ")
	if err != nil {
		Fatalf("Failed to read password: %v", err)
	}
	if confirmation {
		confirm, err := prompt.Stdin.PromptPassword("Repeat password: ")
		if err != nil {
			Fatalf("Failed to read password confirmation: %v", err)
		}
		if password != confirm {
			Fatalf("Passwords do not match")
		}
	}
	return password
}



func GetPassPhraseWithList(text string, confirmation bool, index int, passwords []string) string {
	
	if len(passwords) > 0 {
		if index < len(passwords) {
			return passwords[index]
		}
		return passwords[len(passwords)-1]
	}
	
	password := GetPassPhrase(text, confirmation)
	return password
}
