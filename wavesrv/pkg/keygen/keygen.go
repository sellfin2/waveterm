// Copyright 2023, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Utility functions for generating and reading public/private keypairs.
package keygen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"
)

const p384Params = "BgUrgQQAIg=="

// Creates a keypair with CN=[id], private key at keyFileName, and
// public key certificate at certFileName.
func CreateKeyPair(keyFileName string, certFileName string, id string) error {
	privateKey, err := CreatePrivateKey(keyFileName)
	if err != nil {
		return err
	}
	err = CreateCertificate(certFileName, privateKey, id)
	if err != nil {
		return err
	}
	return nil
}

// Creates a private key at keyFileName (ECDSA, secp384r1 (P-384)), PEM format
func CreatePrivateKey(keyFileName string) (*ecdsa.PrivateKey, error) {
	curve := elliptic.P384() // secp384r1
	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("error generating P-384 key err:%w", err)
	}
	keyFile, err := os.Create(keyFileName)
	if err != nil {
		return nil, fmt.Errorf("error opening file:%s err:%w", keyFileName, err)
	}
	defer keyFile.Close()
	pkBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("error MarshalPKCS8PrivateKey err:%w", err)
	}
	paramsBytes, err := base64.StdEncoding.DecodeString(p384Params)
	if err != nil {
		return nil, fmt.Errorf("error decoding bytes for P-384 EC PARAMETERS err:%w", err)
	}
	var pemParamsBlock = &pem.Block{
		Type:  "EC PARAMETERS",
		Bytes: paramsBytes,
	}
	err = pem.Encode(keyFile, pemParamsBlock)
	if err != nil {
		return nil, fmt.Errorf("error writing EC PARAMETERS pem block err:%w", err)
	}
	var pemPrivateBlock = &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: pkBytes,
	}
	err = pem.Encode(keyFile, pemPrivateBlock)
	if err != nil {
		return nil, fmt.Errorf("error writing EC PRIVATE KEY pem block err:%w", err)
	}
	return privateKey, nil
}

// Creates a public key certificate at certFileName using privateKey with CN=[id].
func CreateCertificate(certFileName string, privateKey *ecdsa.PrivateKey, id string) error {
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1000000000000))
	if err != nil {
		return fmt.Errorf("cannot generate serial number err:%w", err)
	}
	notBefore, err := time.Parse("Jan 2 15:04:05 2006", "Jan 1 00:00:00 2020")
	if err != nil {
		return fmt.Errorf("cannot Parse Date err:%w", err)
	}
	notAfter, err := time.Parse("Jan 2 15:04:05 2006", "Jan 1 00:00:00 2030")
	if err != nil {
		return fmt.Errorf("cannot Parse Date err:%w", err)
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: id,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("error running x509.CreateCertificate err:%v", err)
	}
	certFile, err := os.Create(certFileName)
	if err != nil {
		return fmt.Errorf("error opening file:%s err:%w", certFileName, err)
	}
	defer certFile.Close()
	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	if err != nil {
		return fmt.Errorf("error writing CERTIFICATE pem block err:%w", err)
	}
	return nil
}
