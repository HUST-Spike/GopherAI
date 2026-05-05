package user

import (
	"GopherAI/common/code"
	"GopherAI/dao/user"
	"GopherAI/model"
	"GopherAI/utils"
	"GopherAI/utils/myjwt"
)

func Login(username, password string) (string, code.Code) {
	var userInformation *model.User
	var ok bool
	if ok, userInformation = user.IsExistUser(username); !ok {
		return "", code.CodeUserNotExist
	}
	if userInformation.Password != utils.MD5(password) {
		return "", code.CodeInvalidPassword
	}
	token, err := myjwt.GenerateToken(userInformation.ID, userInformation.Username)
	if err != nil {
		return "", code.CodeServerBusy
	}
	return token, code.CodeSuccess
}

// Register creates a new account using just username + password.
// Email/captcha flow has been removed for the simplified prototype phase.
func Register(username, password string) (string, code.Code) {
	if username == "" || password == "" {
		return "", code.CodeInvalidParams
	}

	if ok, _ := user.IsExistUser(username); ok {
		return "", code.CodeUserExist
	}

	userInformation, ok := user.Register(username, "", password)
	if !ok {
		return "", code.CodeServerBusy
	}

	token, err := myjwt.GenerateToken(userInformation.ID, userInformation.Username)
	if err != nil {
		return "", code.CodeServerBusy
	}
	return token, code.CodeSuccess
}
