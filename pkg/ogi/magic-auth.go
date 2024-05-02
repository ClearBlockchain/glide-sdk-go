package ogi

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/opensaucerer/goaxios"

	"encoding/json"
	"net/http"
	"net/http/cookiejar"

	"github.com/golang-jwt/jwt/v4"

	log "github.com/sirupsen/logrus"
)

type StartVerificationDto struct {
  PhoneNumber string `json:"phoneNumber"`
  Email string `json:"email"`
  FallbackChannel string `json:"fallbackChannel"`
}

type StartVerificationResponseDto struct {
  Type VerificationType `json:"type"`
  AuthUrl string `json:"authUrl"`
}

type VerificationType string

type CheckCodeDto struct {
	PhoneNumber string `json:"phoneNumber"`
	Email string `json:"email"`
	Code string `json:"code"`
}

const (
  MAGIC VerificationType = "MAGIC"
  SMS VerificationType = "SMS"
  EMAIL VerificationType = "EMAIL"
)

func (c *GlideClient) MagicAuth(startVerificationDto *StartVerificationDto) (*StartVerificationResponseDto, error) {
	envConfig, err := ReadEnv()
	if err != nil {
	  return nil, err
	}
  
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %v", err)
	}

	client := &http.Client{
		Jar: jar,
	}
	data, err := json.Marshal(startVerificationDto)
	if err != nil {
		return nil, err
	}
  
	startUrl := fmt.Sprintf("%s/magic-auth/verification/start", envConfig.InternalApiBaseUrl)
	req, err := http.NewRequest("POST", startUrl, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		log.Errorf("failed to start verification request: %v", err)
		return nil, err
	}

	defer res.Body.Close()
  
	if res.StatusCode != http.StatusOK {
		log.Errorf("Error during authentication request: status code %d", res.StatusCode)
		return nil, fmt.Errorf("error during authentication request: status code %d", res.StatusCode)
	}

	var resData StartVerificationResponseDto
	if err := json.NewDecoder(res.Body).Decode(&resData); err != nil {
		log.Errorf("Error parsing verification response: %+v", err)
		return nil, err
	}
  
	// Follow up by requesting to verify the token using the provided auth URL if necessary.
	if resData.Type == MAGIC && resData.AuthUrl != "" {
		authReq, err := http.NewRequest("GET", resData.AuthUrl, nil)
		if err != nil {
			log.Errorf("Error starting magic auth: %v", err)
			return nil, err
		}
	
		authRes, err := client.Do(authReq)
		if err != nil {
			return nil, err
		}
		defer authRes.Body.Close()
	
		if authRes.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to verify auth token: status code %d", authRes.StatusCode)
		}
	
		var jwtString string
		if err := json.NewDecoder(authRes.Body).Decode(&jwtString); err != nil {
			return nil, err
		}
	
		// Parse without validating the signature
		token, _, err := new(jwt.Parser).ParseUnverified(jwtString, jwt.MapClaims{})
		if err != nil {
			return nil, fmt.Errorf("error parsing token: %v", err)
		}
	
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return nil, errors.New("error asserting claims")
		}
	
		if iss, ok := claims["iss"].(string); !ok || iss != envConfig.InternalApiBaseUrl {
			return nil, fmt.Errorf("invalid jwt issuer: expected %v got %v", envConfig.InternalApiBaseUrl, claims["iss"])
		}
	
		// Upon successful verification, update the DTO to reflect this.
		resData.Type = MAGIC
	}
  
	return &resData, nil
  }
  
  func (c *GlideClient) VerifyToken(checkCodeDto *CheckCodeDto) (bool, error) {
	envConfig, err := ReadEnv()
	if err != nil {
	  return false, err
	}
  
	req := goaxios.GoAxios{
	  Url: fmt.Sprintf("%s/magic-auth/verification/check-code", envConfig.InternalApiBaseUrl),
	  Method: "POST",
	  Headers: map[string]string{
		"Content-Type": "application/json",
	  },
	  Body: checkCodeDto,
	}
  
	res := req.RunRest()
	if res.Error != nil {
	  log.Errorf("Error verifying token: %+v", res.Error)
	  return false, res.Error
	}
  
	resData, ok := res.Body.(bool)
	if !ok {
	  log.Errorf("Error parsing token verification response: %+v", res.Error)
	  return false, fmt.Errorf("error parsing token verification response")
	}
  
	return resData, nil
  }