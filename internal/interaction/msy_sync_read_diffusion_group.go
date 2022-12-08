package interaction

import (
	"errors"
	"github.com/golang/protobuf/proto"
	"open_im_sdk/pkg/common"
	"open_im_sdk/pkg/constant"
	"open_im_sdk/pkg/db/db_interface"
	"open_im_sdk/pkg/log"
	"open_im_sdk/pkg/server_api_params"
	"open_im_sdk/pkg/utils"
	"open_im_sdk/sdk_struct"
	"runtime"
	"sync"
	"time"
)

type ReadDiffusionGroupMsgSync struct {
	db_interface.DataBase
	*Ws
	loginUserID              string
	conversationCh           chan common.Cmd2Value
	superGroupMtx            sync.Mutex
	Group2SeqMaxNeedSync     map[string]uint32 //需要同步的seq  可能来自于心跳max，也可能来自于push
	Group2SeqMaxSynchronized map[string]uint32 //已经同步的最大seq
	SuperGroupIDList         []string
	joinedSuperGroupCh       chan common.Cmd2Value
	//	Group2SyncMsgFinished    map[string]bool
}

func NewReadDiffusionGroupMsgSync(dataBase db_interface.DataBase, ws *Ws, loginUserID string, conversationCh chan common.Cmd2Value, joinedSuperGroupCh chan common.Cmd2Value) *ReadDiffusionGroupMsgSync {
	p := &ReadDiffusionGroupMsgSync{DataBase: dataBase, Ws: ws, loginUserID: loginUserID, conversationCh: conversationCh, joinedSuperGroupCh: joinedSuperGroupCh}
	p.Group2SeqMaxNeedSync = make(map[string]uint32, 0)
	p.Group2SeqMaxSynchronized = make(map[string]uint32, 0)
	//	p.Group2SyncMsgFinished = make(map[string]bool, 0)
	go p.updateJoinedSuperGroup()
	return p
}

//协程方式加锁获取读扩散群列表 ok
func (m *ReadDiffusionGroupMsgSync) updateJoinedSuperGroup() {
	for {
		select {
		case cmd := <-m.joinedSuperGroupCh:
			if cmd.Cmd == constant.CmdLogout {
				log.Warn("logout", "close updateJoinedSuperGroup channel ")
				runtime.Goexit()
			} else {
				operationID := cmd.Value.(sdk_struct.CmdJoinedSuperGroup).OperationID
				log.Info(operationID, "updateJoinedSuperGroup cmd: ", cmd)
				g, err := m.GetReadDiffusionGroupIDList()
				if err == nil {
					log.Info(operationID, "GetReadDiffusionGroupIDList, group id list: ", g)
					m.superGroupMtx.Lock()
					m.SuperGroupIDList = g
					m.superGroupMtx.Unlock()
					m.compareSeq(operationID)
				} else {
					log.Error(operationID, "GetReadDiffusionGroupIDList failed ", err.Error())
				}
			}
		}
	}
}

//读取所有的读扩散群id，并加载seq到map中，初始化调用一次， 群列表变化时调用一次  ok
//每个群 记录已经同步的最大seq
func (m *ReadDiffusionGroupMsgSync) compareSeq(operationID string) {
	g, err := m.GetReadDiffusionGroupIDList()
	if err != nil {
		log.Error(operationID, "GetReadDiffusionGroupIDList failed ", err.Error())
		return
	}
	m.superGroupMtx.Lock()
	m.SuperGroupIDList = m.SuperGroupIDList[0:0]
	m.SuperGroupIDList = g
	m.superGroupMtx.Unlock()
	log.Debug(operationID, "compareSeq load groupID list ", m.SuperGroupIDList)

	m.superGroupMtx.Lock()

	defer m.superGroupMtx.Unlock()
	for _, v := range m.SuperGroupIDList {
		n, err := m.GetSuperGroupNormalMsgSeq(v)
		if err != nil {
			log.Error(operationID, "GetSuperGroupNormalMsgSeq failed ", err.Error(), v)
		}
		a, err := m.GetSuperGroupAbnormalMsgSeq(v)
		if err != nil {
			log.Error(operationID, "GetSuperGroupAbnormalMsgSeq failed ", err.Error(), v)
		}
		log.Debug(operationID, "GetSuperGroupNormalMsgSeq GetSuperGroupAbnormalMsgSeq ", n, a, "groupID: ", v)
		var seqMaxSynchronized uint32
		if n > a {
			seqMaxSynchronized = n
		} else {
			seqMaxSynchronized = a
		}
		if seqMaxSynchronized > m.Group2SeqMaxNeedSync[v] {
			m.Group2SeqMaxNeedSync[v] = seqMaxSynchronized
		}
		if seqMaxSynchronized > m.Group2SeqMaxSynchronized[v] {
			m.Group2SeqMaxSynchronized[v] = seqMaxSynchronized
		}
		log.Info(operationID, "load seq, normal, abnormal, ", n, a, m.Group2SeqMaxNeedSync[v], m.Group2SeqMaxSynchronized[v])
	}
}

//处理最大seq消息
func (m *ReadDiffusionGroupMsgSync) doMaxSeq(cmd common.Cmd2Value) {
	operationID := cmd.Value.(sdk_struct.CmdMaxSeqToMsgSync).OperationID
	var groupIDList []string
	//更新需要同步的最大seq 以及SuperGroupIDList
	for groupID, MinMaxSeqOnSvr := range cmd.Value.(sdk_struct.CmdMaxSeqToMsgSync).GroupID2MinMaxSeqOnSvr {
		groupIDList = append(groupIDList, groupID)
		if MinMaxSeqOnSvr.MinSeq > MinMaxSeqOnSvr.MaxSeq {
			log.Warn(operationID, "MinMaxSeqOnSvr.MinSeq > MinMaxSeqOnSvr.MaxSeq", MinMaxSeqOnSvr.MinSeq, MinMaxSeqOnSvr.MaxSeq)
			return
		}
		if MinMaxSeqOnSvr.MaxSeq > m.Group2SeqMaxNeedSync[groupID] {
			m.Group2SeqMaxNeedSync[groupID] = MinMaxSeqOnSvr.MaxSeq
		}
		if MinMaxSeqOnSvr.MinSeq > m.Group2SeqMaxSynchronized[groupID] {
			m.Group2SeqMaxSynchronized[groupID] = MinMaxSeqOnSvr.MinSeq - 1
		}
	}
	m.superGroupMtx.Lock()
	m.SuperGroupIDList = m.SuperGroupIDList[0:0]
	m.SuperGroupIDList = groupIDList
	m.superGroupMtx.Unlock()

	//同步最新消息，内部保证只调用一次
	//m.syncLatestMsg(operationID)

	//同步所有群的新消息
	m.syncMsgFroAllGroup(operationID)
}

//在获取最大seq后同步最新消息，只调用一次 ok
//func (m *ReadDiffusionGroupMsgSync) syncLatestMsg(operationID string) {
//	m.superGroupMtx.Lock()
//	flag := 0
//	for _, v := range m.SuperGroupIDList {
//		if m.Group2SyncMsgFinished[v] == false {
//			flag = 1
//			break
//		}
//	}
//
//	if flag == 1 {
//		log.Info(operationID, "sync latest msg begin for read diffusion group: ", m.SuperGroupIDList)
//		//	m.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncBegin)
//		for _, v := range m.SuperGroupIDList {
//			m.syncLatestMsgForGroup(v, operationID, constant.MsgSyncProcessing)
//		}
//		//	m.TriggerCmdNewMsgCome(nil, operationID, constant.MsgSyncEnd)
//		log.Info(operationID, "sync latest msg end for read diffusion group: ", m.SuperGroupIDList)
//	} else {
//		log.Info(operationID, "do nothing ")
//		for _, v := range m.SuperGroupIDList {
//			m.syncLatestMsgForGroup(v, operationID, 0)
//		}
//	}
//
//	//end
//	m.superGroupMtx.Unlock()
//}

//同步某个群的最新消息(最多PullMsgNumForReadDiffusion条)
func (m *ReadDiffusionGroupMsgSync) syncLatestLimitedMsgForGroup(groupID, operationID string, loginSync int) error {
	log.NewDebug(operationID, utils.GetSelfFuncName(), " start ", groupID, loginSync, m.Group2SeqMaxNeedSync[groupID], m.Group2SeqMaxSynchronized[groupID])
	end := m.Group2SeqMaxNeedSync[groupID]
	synchronized := m.Group2SeqMaxSynchronized[groupID]
	begin := synchronized + 1

	if int64(end)-int64(synchronized) > int64(constant.PullMsgNumForReadDiffusion) {
		begin = end - uint32(constant.PullMsgNumForReadDiffusion) + 1
	}

	log.Debug(operationID, "syncLatestMsgForGroup seq: ", "begin: ", begin, " end: ", end, " synchronized ", synchronized, groupID)
	if begin > end {
		log.Debug(operationID, "do nothing syncLatestMsgForGroup seq: ", end, synchronized, begin, groupID)
		return nil
	}
	if err := m.syncMsgFromServer(begin, end, groupID, operationID, loginSync); err != nil {
		return utils.Wrap(err, "syncMsgFromServer failed ")
	}
	m.Group2SeqMaxSynchronized[groupID] = end
	return nil
}

//获取某个群的最新消息，只调用一次
//func (m *ReadDiffusionGroupMsgSync) syncLatestMsgForGroup(groupID, operationID string, loginSync int) {
//	log.NewDebug(operationID, utils.GetSelfFuncName(), "syncLatestMsgForGroup start", groupID, loginSync, m.Group2SyncMsgFinished[groupID], m.Group2SeqMaxNeedSync[groupID], m.Group2SeqMaxSynchronized[groupID])
//	if !m.Group2SyncMsgFinished[groupID] {
//		need := m.Group2SeqMaxNeedSync[groupID]
//		synchronized := m.Group2SeqMaxSynchronized[groupID]
//		begin := synchronized + 1
//		if int64(need)-int64(synchronized) > int64(constant.PullMsgNumForReadDiffusion) {
//			begin = need - uint32(constant.PullMsgNumForReadDiffusion) + 1
//		}
//		m.Group2SyncMsgFinished[groupID] = true
//		log.Debug(operationID, "syncLatestMsgForGroup seq: ", need, synchronized, begin)
//		if begin > need {
//			log.Debug(operationID, "do nothing syncLatestMsgForGroup seq: ", need, synchronized, begin)
//			return
//		}
//		m.syncMsgFromServer(begin, need, groupID, operationID, loginSync)
//
//		m.Group2SeqMaxSynchronized[groupID] = begin
//	}
//}

func (m *ReadDiffusionGroupMsgSync) doPushMsg(cmd common.Cmd2Value) {
	msg := cmd.Value.(sdk_struct.CmdPushMsgToMsgSync).Msg
	operationID := cmd.Value.(sdk_struct.CmdPushMsgToMsgSync).OperationID
	log.Debug(operationID, "recv super group push msg, doPushMsg ", msg.Seq, msg.ServerMsgID, msg.ClientMsgID, msg.GroupID, msg.SessionType, m.Group2SeqMaxSynchronized[msg.GroupID], m.Group2SeqMaxNeedSync[msg.GroupID], m.loginUserID)
	if msg.Seq == 0 {
		m.TriggerCmdNewMsgCome([]*server_api_params.MsgData{msg}, operationID, 0)
		return
	}
	if msg.Seq == m.Group2SeqMaxSynchronized[msg.GroupID]+1 {
		log.Debug(operationID, "TriggerCmdNewMsgCome ", msg.ServerMsgID, msg.ClientMsgID, msg.Seq)
		m.TriggerCmdNewMsgCome([]*server_api_params.MsgData{msg}, operationID, 0)
		m.Group2SeqMaxSynchronized[msg.GroupID] = msg.Seq
	}

	if msg.Seq > m.Group2SeqMaxNeedSync[msg.GroupID] {
		m.Group2SeqMaxNeedSync[msg.GroupID] = msg.Seq
	}
	log.Debug(operationID, "syncMsgFromServer ", m.Group2SeqMaxSynchronized[msg.GroupID]+1, m.Group2SeqMaxNeedSync[msg.GroupID])
	//获取此群最新消息，内部保证只调用一次
	m.syncLatestLimitedMsgForGroup(msg.GroupID, operationID, 0)
	//同步此群新消息
	//m.syncMsgForOneGroup(operationID, msg.GroupID)
}

//同步所有群的新消息(最多PullMsgNumForReadDiffusion)
func (m *ReadDiffusionGroupMsgSync) syncMsgFroAllGroup(operationID string) error {
	m.superGroupMtx.Lock()
	defer m.superGroupMtx.Unlock()
	for _, v := range m.SuperGroupIDList {
		log.Info(operationID, "do syncLatestLimitedMsgForGroup ", v)
		if err := m.syncLatestLimitedMsgForGroup(v, operationID, 0); err != nil {
			return utils.Wrap(err, "syncLatestLimitedMsgForGroup failed")
		}
	}
	return nil
}

//同步某个群的新消息 ok
func (m *ReadDiffusionGroupMsgSync) syncMsgForOneGroup(operationID string, groupID string) {
	log.NewDebug(operationID, utils.GetSelfFuncName(), "syncMsgForOneGroup start", groupID)

	inGroup := false
	for _, v := range m.SuperGroupIDList {
		if groupID != "" && v != groupID {
			continue
		} else {
			inGroup = true
			break
		}
	}
	if !inGroup {
		m.superGroupMtx.Lock()
		m.SuperGroupIDList = append(m.SuperGroupIDList, groupID)
		m.superGroupMtx.Unlock()
	}

	m.superGroupMtx.Lock()
	for _, v := range m.SuperGroupIDList {
		if groupID != "" && v != groupID {
			continue
		}
		seqMaxNeedSync := m.Group2SeqMaxNeedSync[v]
		seqMaxSynchronized := m.Group2SeqMaxSynchronized[v]
		//限制100条
		if seqMaxNeedSync > seqMaxSynchronized {
			log.Info(operationID, "do syncMsg ", seqMaxSynchronized+1, seqMaxNeedSync)
			m.syncMsgFromServer(seqMaxSynchronized+1, seqMaxNeedSync, v, operationID, 0)
			m.Group2SeqMaxSynchronized[v] = seqMaxNeedSync
		} else {
			log.NewDebug(operationID, "msg not sync", seqMaxNeedSync, seqMaxSynchronized)
		}
		break
	}
	m.superGroupMtx.Unlock()
	log.NewDebug(operationID, utils.GetSelfFuncName(), "syncMsgForOneGroup end", groupID)
}

func (m *ReadDiffusionGroupMsgSync) syncMsgFromServer(beginSeq, endSeq uint32, groupID, operationID string, loginSync int) error {
	log.Debug(operationID, utils.GetSelfFuncName(), "args: ", beginSeq, endSeq, groupID)
	if beginSeq > endSeq {
		log.Error(operationID, "beginSeq > endSeq", beginSeq, endSeq)
		return nil
	}

	var needSyncSeqList []uint32
	for i := beginSeq; i <= endSeq; i++ {
		needSyncSeqList = append(needSyncSeqList, i)
	}
	var SPLIT = constant.SplitPullMsgNum
	for i := 0; i < len(needSyncSeqList)/SPLIT; i++ {
		if err := m.syncMsgFromServerSplit(needSyncSeqList[i*SPLIT:(i+1)*SPLIT], groupID, operationID, loginSync); err != nil {
			return utils.Wrap(err, "syncMsgFromServerSplit failed")
		}
	}
	if err := m.syncMsgFromServerSplit(needSyncSeqList[SPLIT*(len(needSyncSeqList)/SPLIT):], groupID, operationID, loginSync); err != nil {
		return utils.Wrap(err, "syncMsgFromServerSplit failed")
	}
	return nil
}

func (m *ReadDiffusionGroupMsgSync) syncMsgFromServerSplit(needSyncSeqList []uint32, groupID, operationID string, loginSync int) error {
	var pullMsgReq server_api_params.PullMessageBySeqListReq
	pullMsgReq.UserID = m.loginUserID
	pullMsgReq.GroupSeqList = make(map[string]*server_api_params.SeqList, 0)
	pullMsgReq.GroupSeqList[groupID] = &server_api_params.SeqList{SeqList: needSyncSeqList}
	tryNum := 0
	tryMaxNum := 10
	for {
		if tryNum != 0 {
			time.Sleep(3 * time.Second)
		}
		if tryNum == tryMaxNum {
			return errors.New("try many times ")
		}
		pullMsgReq.OperationID = operationID
		log.Debug(operationID, "read diffusion group pull message, req: ", pullMsgReq)
		resp, err := m.SendReqWaitResp(&pullMsgReq, constant.WSPullMsgBySeqList, 60, 1, m.loginUserID, operationID)
		if err != nil && m.LoginStatus() == constant.Logout {
			log.Error(operationID, "SendReqWaitResp failed  Logout status ", err.Error(), m.LoginStatus())
			return utils.Wrap(err, " SendReqWaitResp failed Logout status")
		}
		if err != nil {
			log.Error(operationID, "SendReqWaitResp failed,  constant.MsgSyncFailed ", err.Error(), constant.WSPullMsgBySeqList, 60, 1, m.loginUserID)
			tryNum++
			continue
		}

		var pullMsgResp server_api_params.PullMessageBySeqListResp
		err = proto.Unmarshal(resp.Data, &pullMsgResp)
		if err != nil {
			log.Error(operationID, "pullMsgResp Unmarshal failed ", err.Error())
			return utils.Wrap(err, " Unmarshal failed ")
		}
		log.Debug(operationID, "syncMsgFromServerSplit pull msg ", pullMsgReq.String(), pullMsgResp.String())
		for _, v := range pullMsgResp.GroupMsgDataList {
			log.Debug(operationID, "TriggerCmdNewMsgCome ", len(v.MsgDataList))
			m.TriggerCmdNewMsgCome(v.MsgDataList, operationID, loginSync)
		}
		return nil
	}
}

func (m *ReadDiffusionGroupMsgSync) TriggerCmdNewMsgCome(msgList []*server_api_params.MsgData, operationID string, loginSync int) {
	for {
		err := common.TriggerCmdSuperGroupMsgCome(sdk_struct.CmdNewMsgComeToConversation{MsgList: msgList, OperationID: operationID, SyncFlag: loginSync}, m.conversationCh)
		if err != nil {
			log.Warn(operationID, "TriggerCmdSuperGroupMsgCome failed ", err.Error(), m.loginUserID)
			continue
		}
		log.Info(operationID, "TriggerCmdSuperGroupMsgCome ok ", m.loginUserID, loginSync)
		return
	}
}

func (m *ReadDiffusionGroupMsgSync) DoConnectSuccess(GroupMaxAndMinSeq map[string]*server_api_params.MaxAndMinSeq, operationID string) error {
	var groupIDList []string
	//更新需要同步的最大seq 以及SuperGroupIDList
	for groupID, MinMaxSeqOnSvr := range GroupMaxAndMinSeq {
		groupIDList = append(groupIDList, groupID)
		if MinMaxSeqOnSvr.MinSeq > MinMaxSeqOnSvr.MaxSeq {
			log.Warn(operationID, "MinMaxSeqOnSvr.MinSeq > MinMaxSeqOnSvr.MaxSeq", MinMaxSeqOnSvr.MinSeq, MinMaxSeqOnSvr.MaxSeq)
			return errors.New("MinMaxSeqOnSvr.MinSeq > MinMaxSeqOnSvr.MaxSeq")
		}
		if MinMaxSeqOnSvr.MaxSeq > m.Group2SeqMaxNeedSync[groupID] {
			m.Group2SeqMaxNeedSync[groupID] = MinMaxSeqOnSvr.MaxSeq
		}
		if MinMaxSeqOnSvr.MinSeq > m.Group2SeqMaxSynchronized[groupID] {
			m.Group2SeqMaxSynchronized[groupID] = MinMaxSeqOnSvr.MinSeq - 1
		}
	}
	m.superGroupMtx.Lock()
	m.SuperGroupIDList = m.SuperGroupIDList[0:0]
	m.SuperGroupIDList = groupIDList
	m.superGroupMtx.Unlock()

	//同步所有群的新消息
	return m.syncMsgFroAllGroup(operationID)
}
