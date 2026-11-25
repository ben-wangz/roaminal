package dbbootstrap

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"
)

const ownerAnnotation = "roaminal.io/postgres-bootstrap-owner"
const secretType = "kubernetes.io/basic-auth"

func bootstrap(ctx context.Context, client *apiClient, c Config) error {
	const root = "/api/v1/namespaces/"
	secrets := root + c.Namespace + "/secrets"
	markers := root + c.Namespace + "/configmaps"
	secretPath := secrets + "/" + c.Secret
	owner := map[string]string{ownerAnnotation: c.Owner}
	marker := resource{APIVersion: "v1", Kind: "ConfigMap", Metadata: metadata{Name: c.Marker, Annotations: owner}}
	secret, err := client.request(ctx, http.MethodGet, secretPath, nil)
	if err == nil {
		if err := verifySecret(secret, c); err != nil {
			return err
		}
		// Also protect credentials restored without the marker, without
		// updating either existing object.
		return verifyOrRestoreMarker(ctx, client, markers, marker)
	}
	if !errors.Is(err, errNotFound) {
		return err
	}

	reservation, err := client.request(ctx, http.MethodPost, markers, &marker)
	if errors.Is(err, errExists) {
		reservation, err = client.request(ctx, http.MethodGet, markers+"/"+c.Marker, nil)
		if err != nil || !owned(reservation, c.Marker, c.Owner) {
			return errors.New("PostgreSQL bootstrap reservation could not be verified")
		}
		return waitForSecret(ctx, client, secretPath, c)
	}
	if err != nil {
		return err
	}
	if !owned(reservation, c.Marker, c.Owner) {
		return errors.New("PostgreSQL bootstrap reservation could not be verified")
	}
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return errors.New("generate PostgreSQL bootstrap password")
	}
	password := base64.RawURLEncoding.EncodeToString(data[:])
	secret = resource{APIVersion: "v1", Kind: "Secret", Type: secretType, Metadata: metadata{Name: c.Secret, Annotations: owner},
		StringData: map[string]string{"username": c.Username, "password": password, c.DSNKey: c.dsn(password)}}
	saved, err := client.request(ctx, http.MethodPost, secrets, &secret)
	if errors.Is(err, errExists) {
		saved, err = client.request(ctx, http.MethodGet, secretPath, nil)
	}
	if err != nil {
		return err
	}
	return verifySecret(saved, c)
}

func verifyOrRestoreMarker(ctx context.Context, client *apiClient, markers string, expected resource) error {
	path := markers + "/" + expected.Metadata.Name
	actual, err := client.request(ctx, http.MethodGet, path, nil)
	if errors.Is(err, errNotFound) {
		actual, err = client.request(ctx, http.MethodPost, markers, &expected)
		if errors.Is(err, errExists) {
			actual, err = client.request(ctx, http.MethodGet, path, nil)
		}
	}
	if err != nil {
		return err
	}
	if !owned(actual, expected.Metadata.Name, expected.Metadata.Annotations[ownerAnnotation]) {
		return errors.New("PostgreSQL bootstrap reservation could not be verified")
	}
	return nil
}

func owned(r resource, name, owner string) bool {
	return r.Metadata.Name == name && r.Metadata.Annotations[ownerAnnotation] == owner
}

func verifySecret(secret resource, c Config) error {
	username, userErr := base64.StdEncoding.DecodeString(secret.Data["username"])
	password, passwordErr := base64.StdEncoding.DecodeString(secret.Data["password"])
	dsn, dsnErr := base64.StdEncoding.DecodeString(secret.Data[c.DSNKey])
	if !owned(secret, c.Secret, c.Owner) || secret.Type != secretType || userErr != nil || passwordErr != nil || dsnErr != nil ||
		string(username) != c.Username || len(password) == 0 || string(dsn) != c.dsn(string(password)) {
		return errors.New("existing PostgreSQL bootstrap Secret does not match the managed database; restore matching credentials")
	}
	return nil
}

func waitForSecret(ctx context.Context, client *apiClient, path string, c Config) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		secret, err := client.request(ctx, http.MethodGet, path, nil)
		if err == nil {
			return verifySecret(secret, c)
		}
		if !errors.Is(err, errNotFound) {
			return err
		}
		select {
		case <-ctx.Done():
			return errors.New("PostgreSQL bootstrap was reserved but its Secret is missing; restore it before retrying")
		case <-tick.C:
		}
	}
}
