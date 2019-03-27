// Package vartime provides a insecure/slow variable time Deoxys-II-256-128
// implementation.
//
// WARNING: THIS IMPLEMENTATION MUST NOT BE USED FOR ANYTHING REQUIRING
// ACTUAL SECURITY.
package vartime

import (
	"crypto/subtle"

	"github.com/oasislabs/deoxysii/internal/api"
)

var Impl api.Impl = &vartimeImpl{}

type vartimeImpl struct{}

func (impl *vartimeImpl) Name() string {
	return "vartime"
}

func (impl *vartimeImpl) STKDeriveK(key []byte, derivedKs *[api.STKCount][api.STKSize]byte) {
	api.STKDeriveK(key, derivedKs)
}

func (impl *vartimeImpl) E(derivedKs *[api.STKCount][api.STKSize]byte, nonce, dst, ad, msg []byte) {
	var (
		tweak [api.TweakSize]byte
		tmp   [api.BlockSize]byte
		i, j  int
	)

	// Associated data.
	adLen := len(ad)
	var auth [api.TagSize]byte
	for i = 0; adLen >= api.BlockSize; i++ {
		// 5. Auth <- Auth ^ Ek(0010||i, Ai+1)
		api.EncodeTagTweak(&tweak, api.PrefixADBlock, i)
		bcEncrypt(tmp[:], derivedKs, &tweak, ad[i*16:])
		api.XORBytes(auth[:], auth[:], tmp[:], 16)
		adLen -= api.BlockSize
	}
	if adLen > 0 {
		// 8. Auth <- Auth ^ Ek(0110||la, pad10*(A*))
		api.EncodeTagTweak(&tweak, api.PrefixADFinal, i)

		var aStar [16]byte
		copy(aStar[:], ad[len(ad)-adLen:])
		aStar[adLen] = 0x80

		bcEncrypt(tmp[:], derivedKs, &tweak, aStar[:])
		api.XORBytes(auth[:], auth[:], tmp[:], 16)
	}

	// Message authentication and tag generation.
	msgLen := len(msg)
	tag := auth[:]
	for j = 0; msgLen >= api.BlockSize; j++ {
		// 15. tag <- tag ^ Ek(0000||j, Mj+1)
		api.EncodeTagTweak(&tweak, api.PrefixMsgBlock, j)
		bcEncrypt(tmp[:], derivedKs, &tweak, msg[j*16:])
		api.XORBytes(tag, tag, tmp[:], 16)
		msgLen -= api.BlockSize
	}
	if msgLen > 0 {
		// 18. tag <- tag & Ek(0100||l, pad10*(M*))
		api.EncodeTagTweak(&tweak, api.PrefixMsgFinal, j)

		var mStar [16]byte
		copy(mStar[:], msg[len(msg)-msgLen:])
		mStar[msgLen] = 0x80

		bcEncrypt(tmp[:], derivedKs, &tweak, mStar[:])
		api.XORBytes(tag, tag, tmp[:], 16)
	}

	// 20. tag <- Ek(0001||0000||N, tag)
	var encNonce [api.BlockSize]byte
	copy(encNonce[1:], nonce[:])
	encNonce[0] = api.PrefixTag << api.PrefixShift
	bcEncrypt(tag, derivedKs, &encNonce, tag)

	// Message encryption.
	var encBlk [api.BlockSize]byte
	encNonce[0] = 0 // 0x00 || nonce
	c := dst[0:]
	msgLen = len(msg)
	for j = 0; msgLen >= api.BlockSize; j++ {
		// 24. Cj <- Mj ^ Ek(1||tag^j, 00000000||N)
		api.EncodeEncTweak(&tweak, tag, j)
		bcEncrypt(encBlk[:], derivedKs, &tweak, encNonce[:])
		api.XORBytes(c[j*16:], msg[j*16:], encBlk[:], 16)
		msgLen -= api.BlockSize
	}
	if msgLen > 0 {
		// 24. C* <- M* ^ Ek(1||tag^l, 00000000||N)
		api.EncodeEncTweak(&tweak, tag, j)
		bcEncrypt(encBlk[:], derivedKs, &tweak, encNonce[:])
		api.XORBytes(c[j*16:], msg[j*16:], encBlk[:], msgLen)
	}

	// Append the tag.
	copy(dst[len(dst)-api.TagSize:], tag)
}

func (impl *vartimeImpl) D(derivedKs *[api.STKCount][api.STKSize]byte, nonce, dst, ad, ct []byte) bool {
	// Split out ct into ciphertext and tag.
	ctLen := len(ct) - api.TagSize
	ciphertext, tag := ct[:ctLen], ct[ctLen:]

	var j int

	// Message decryption.
	var (
		decTweak         [api.TweakSize]byte
		decBlk, decNonce [api.BlockSize]byte
	)
	copy(decNonce[1:], nonce) // 0x00 || nonce
	for j = 0; ctLen >= api.BlockSize; j++ {
		// 4. Mj <- Cj ^ Ek(1||tag^j, 00000000||N)
		api.EncodeEncTweak(&decTweak, tag, j)
		bcEncrypt(decBlk[:], derivedKs, &decTweak, decNonce[:])
		api.XORBytes(dst[j*16:], ciphertext[j*16:], decBlk[:], 16)
		ctLen -= api.BlockSize
	}
	if ctLen > 0 {
		// 7. M* <- C* ^ Ek(1||tag^l, 00000000||N)
		api.EncodeEncTweak(&decTweak, tag, j)
		bcEncrypt(decBlk[:], derivedKs, &decTweak, decNonce[:])
		api.XORBytes(dst[j*16:], ciphertext[j*16:], decBlk[:], ctLen)
	}

	// Associated data.
	adLen := len(ad)
	var (
		auth  [api.TagSize]byte
		tweak [api.TweakSize]byte
		tmp   [api.BlockSize]byte
		i     int
	)
	for i = 0; adLen >= api.BlockSize; i++ {
		// 14. Auth <- Auth ^ Ek(0010||i, Ai+1)
		api.EncodeTagTweak(&tweak, api.PrefixADBlock, i)
		bcEncrypt(tmp[:], derivedKs, &tweak, ad[i*16:])
		api.XORBytes(auth[:], auth[:], tmp[:], 16)
		adLen -= api.BlockSize
	}
	if adLen > 0 {
		// 17. Auth <- Auth ^ Ek(0110||la, pad10*(A*))
		api.EncodeTagTweak(&tweak, api.PrefixADFinal, i)

		var aStar [16]byte
		copy(aStar[:], ad[len(ad)-adLen:])
		aStar[adLen] = 0x80

		bcEncrypt(tmp[:], derivedKs, &tweak, aStar[:])
		api.XORBytes(auth[:], auth[:], tmp[:], 16)
	}

	// Message authentication and tag generation.
	msgLen := len(dst)
	tagP := auth[:]
	for j = 0; msgLen >= api.BlockSize; j++ {
		// 24. tag' <- tag' ^ Ek(0000||j, Mj+1)
		api.EncodeTagTweak(&tweak, api.PrefixMsgBlock, j)
		bcEncrypt(tmp[:], derivedKs, &tweak, dst[j*16:])
		api.XORBytes(tagP, tagP, tmp[:], 16)
		msgLen -= api.BlockSize
	}
	if msgLen > 0 {
		// 27. tag <- tag & Ek(0100||l, pad10*(M*))
		api.EncodeTagTweak(&tweak, api.PrefixMsgFinal, j)

		var mStar [16]byte
		copy(mStar[:], dst[len(dst)-msgLen:])
		mStar[msgLen] = 0x80

		bcEncrypt(tmp[:], derivedKs, &tweak, mStar[:])
		api.XORBytes(tagP, tagP, tmp[:], 16)
	}

	// 29. tag' <- Ek(0001||0000||N, tag')
	decNonce[0] = api.PrefixTag << api.PrefixShift
	bcEncrypt(tagP, derivedKs, &decNonce, tagP)

	// Tag verification.
	return subtle.ConstantTimeCompare(tag, tagP) == 1
}
