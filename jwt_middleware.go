package main

import (
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"net/http"
)

func jwtMiddleware(next http.Handler) http.Handler {
	AuthSecret := "django-insecure-9t*0n5hv%jc@jt33c19c@=z8-w!087_9ghz)!nh3^89viir^u*"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authToken := r.URL.Query().Get("auth_token")

		if authToken == "" {

			next.ServeHTTP(w, r)
		}

		claims := jwt.MapClaims{}

		token, err := jwt.ParseWithClaims(authToken, &claims, func(token *jwt.Token) (interface{}, error) {
			method := token.Method.Alg()
			if method != "HS256" {
				return nil, errors.New("unexpected algorithm")
			}
			return []byte(AuthSecret), nil
		})
		ID := fmt.Sprintf("%v", claims["user_id"])

		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		if !token.Valid {
			_, _ = w.Write([]byte("Token is invalid"))
			return
		}

		r.Header.Add("ID", ID)
		next.ServeHTTP(w, r)
	})
}
