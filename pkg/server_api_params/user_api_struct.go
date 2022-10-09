package server_api_params

type GetUsersInfoReq struct {
	OperationID string   `json:"operationID" binding:"required"`
	UserIDList  []string `json:"userIDList" binding:"required"`
}
type GetUsersInfoResp struct {
	CommResp
	UserInfoList []*PublicUserInfo
	Data         []map[string]interface{} `json:"data"`
}

type UpdateSelfUserInfoReq struct {
	ApiUserInfo
	OperationID string `json:"operationID" binding:"required"`
}

type UpdateUserInfoResp struct {
	CommResp
}
type SetGlobalRecvMessageOptReq struct {
	OperationID      string `json:"operationID" binding:"required"`
	GlobalRecvMsgOpt *int32 `json:"globalRecvMsgOpt" binding:"omitempty,oneof=0 1 2"`
}
type SetGlobalRecvMessageOptResp struct {
	CommResp
}
type GetSelfUserInfoReq struct {
	OperationID string `json:"operationID" binding:"required"`
	UserID      string `json:"userID" binding:"required"`
}
type GetSelfUserInfoResp struct {
	CommResp
	UserInfo *UserInfo              `json:"-"`
	Data     map[string]interface{} `json:"data"`
}

type GetEncryptionKeyReq struct {
	OperationID string `json:"operationID" binding:"required"`
	UserID      string `json:"userID"`
	GroupID     string `json:"groupID"`
	KeyVersion  int32  `json:"keyVersion"`
}
type VersionKey struct {
	Version int32  `json:"version"`
	Key     string `json:"key"`
}

type GetEncryptionKeyResp struct {
	CommResp
	VersionKeyList []*VersionKey            `json:"-"`
	Data           []map[string]interface{} `json:"data" swaggerignore:"true"`
}

type GetAllJoinedGroupEncryptionKeyReq struct {
	OperationID string `json:"operationID" binding:"required"`
	UserID      string `json:"userID"`
}

type GroupVersionKey struct {
	Version int32  `json:"version"`
	Key     string `json:"key"`
	GroupID string `json:"groupID"`
}

type GetAllJoinedGroupEncryptionKeyResp struct {
	CommResp
	GroupVersionKeyList []*GroupVersionKey       `json:"-"`
	Data                []map[string]interface{} `json:"data" swaggerignore:"true"`
}
