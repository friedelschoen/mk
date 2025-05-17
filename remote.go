package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// for files that are http(s) urls, use the Last-Modified header
func updateHttpTimestamp(u *node) {
	resp, err := http.Head(u.name)
	if err != nil {
		log.Fatal(err)
	}
	lastModified := resp.Header.Get("Last-Modified")
	if lastModified == "" {
		// no Last-Modified header so lets assume that it
		// is very old - but still exists
		u.t = time.Unix(0, 0)
		u.exists = true
		u.flags |= nodeFlagProbable
	} else {
		tmptime, err := time.Parse(time.RFC1123, lastModified)
		if err != nil {
			log.Fatal(err)
		}
		u.t = tmptime
		u.exists = true
		u.flags |= nodeFlagProbable
	}
}

func updateS3Timestamp(u *node, uri *url.URL) {
	ses, err := session.NewSession()
	if err != nil {
		panic(fmt.Errorf("unable to create a session: %w", err))
	}
	svc := s3.New(ses)
	input := &s3.HeadObjectInput{
		Bucket: aws.String(uri.Host),
		Key:    aws.String(uri.Path[1:]),
	}

	result, err := svc.HeadObject(input)
	if err != nil {
		u.t = time.Unix(0, 0)
		u.exists = false

	} else {
		u.t = *result.LastModified
		u.exists = true
	}
	u.flags |= nodeFlagProbable
	//fmt.Println(result)
}
