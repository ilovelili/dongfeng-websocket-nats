package websocketnats

import (
	"errors"
	"fmt"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/lestrrat-go/jwx/jwk"
)

var (
	// JWKS jwks jwks https://auth0.com/docs/jwks
	JWKS string
)

// ParseJWT parse json web token and output claims and token
func ParseJWT(idtoken string, jwks string) (claims jwt.MapClaims, token *jwt.Token, err error) {
	claims = jwt.MapClaims{}
	JWKS = jwks
	token, err = jwt.ParseWithClaims(idtoken, claims, getKey)
	return
}

func getKey(token *jwt.Token) (interface{}, error) {
	// validate the alg
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
	}

	keyID, ok := token.Header["kid"].(string)
	if !ok {
		return nil, errors.New("expecting JWT header to have string kid")
	}

	keySet, err := jwk.FetchHTTP(JWKS)
	if err != nil {
		return nil, err
	}

	if key := keySet.LookupKeyID(keyID); len(key) == 1 {
		return key[0].Materialize()
	}

	return nil, errors.New("unable to find key")
}

// ResolveIDToken resolve id_token saved in header by removing the "bearer " rpefix
func ResolveIDToken(token string) (idtoken string, valid bool) {
	valid = true
	idtokensegments := strings.Split(token, "Bearer ")
	if len(idtokensegments) != 2 {
		valid = false
		return
	}

	idtoken = idtokensegments[1]
	return
}
