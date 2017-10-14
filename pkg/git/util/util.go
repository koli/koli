package util

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
)

const (
	releasePrefix = "releases"
	repoPrefix    = "repos"
)

// DecodeUserToken decodes a jwtToken (HS256 and RS256) into a *platform.User
func DecodeUserToken(jwtTokenString, jwtSecret string, rawPubKey []byte) (*platform.User, error) {
	user := &platform.User{}
	token, err := jwt.ParseWithClaims(jwtTokenString, user, func(token *jwt.Token) (interface{}, error) {
		switch t := token.Method.(type) {
		case *jwt.SigningMethodRSA:
			if rawPubKey == nil {
				return nil, fmt.Errorf("missing public key")
			}
			pubKey, err := jwt.ParseRSAPublicKeyFromPEM(rawPubKey)
			if err != nil {
				log.Fatalf("failed parsing raw public key [%v]", err)
			}
			return pubKey, nil
		case *jwt.SigningMethodHMAC:
			return []byte(jwtSecret), nil
		default:
			return nil, fmt.Errorf("unknown sign method [%v]", t)
		}
	})
	if err == nil && token.Valid {
		return user, nil
	}
	switch t := err.(type) {
	case *jwt.ValidationError:
		if t.Errors&jwt.ValidationErrorMalformed != 0 {
			return nil, fmt.Errorf("it's not a valid token")
		} else if t.Errors&(jwt.ValidationErrorExpired|jwt.ValidationErrorNotValidYet) != 0 {
			return nil, fmt.Errorf("the token is expired or not valid yet")
		} else {
			return nil, fmt.Errorf("failed decoding token [%v]", err)
		}
	default:
		return nil, fmt.Errorf("unknown error, failed decoding token [%v]", err)
	}
}

// CreateRepo returns a bool indicating whether a folder is created (true) or already
// existed (false).
func createRepo(folderPath string, gitInit bool, createLock *sync.Mutex) (bool, error) {
	createLock.Lock()
	defer createLock.Unlock()

	fi, err := os.Stat(folderPath)
	if err == nil && fi.IsDir() {
		// Nothing to do.
		log.Printf("Directory %s already exists.", folderPath)
		return false, nil
	} else if os.IsNotExist(err) {
		log.Printf("Creating new directory at %s", folderPath)
		// Create directory
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			log.Printf("Failed to create repository: %s", err)
			return false, err
		}
		if gitInit {
			cmd := exec.Command("git", "init", "--bare")
			cmd.Dir = folderPath
			if out, err := cmd.CombinedOutput(); err != nil {
				log.Printf("git init output: %s", out)
				return false, err
			}
		}
		return true, nil
	} else if err == nil {
		return false, errors.New("Expected directory, found file.")
	}
	return false, err
}

type gitURL struct {
	addr  *url.URL
	user  string
	token string
	repo  string
}

func (u *gitURL) String() string {
	return fmt.Sprintf("%s://%s/%s", u.addr.Scheme, u.addr.Host, u.repo) + ".git"
}

// WithCredentials constructs the git URL with the provided credentials
func (u *gitURL) WithCredentials() string {
	credentials := ""
	if u.token != "" {
		credentials = fmt.Sprintf(":%s@", u.token)
	}
	return fmt.Sprintf("%s://%s%s/%s", u.addr.Scheme, credentials, u.addr.Host, u.repo+".git")
}

type releaseURL struct {
	addr string
}

// String returns the original value
func (r *releaseURL) String() string {
	return r.addr
}

// WithRevision suffix the end of the url with the given revision
func (r *releaseURL) WithRevision(revision string) string {
	return fmt.Sprintf("%s/%s", r.addr, revision)
}

// GetKubernetesClient returns a new client to interact with the api server
func GetKubernetesClient(kubernetesHost string) (c *kubernetes.Clientset, err error) {
	var config *rest.Config
	if len(kubernetesHost) == 0 {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error creating client configuration: %v", err)
		}
	} else {
		config = &rest.Config{Host: kubernetesHost}
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

// GenerateRandomBytes returns securely generated random string.
// It will return an error if the system's secure random
// number generator fails to function correctly, in which
// case the caller should not continue.
func GenerateRandomBytes(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateNewJwtToken creates a new user token to allow machine-to-machine interaction
func GenerateNewJwtToken(key, customer, org string, tokenType platform.TokenType) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := make(jwt.MapClaims)
	// Set some claims
	claims["customer"] = customer
	claims["org"] = org
	claims["kolihub.io/type"] = tokenType
	// A system token doesn't expire, make sure this token have limited
	// access to API's
	// claims["exp"] = time.Now().UTC().Add(time.Minute * 20).Unix()
	claims["iat"] = time.Now().UTC().Unix()
	token.Claims = claims

	// Sign and get the complete encoded token as a string
	return token.SignedString(bytes.NewBufferString(key).Bytes())
}
