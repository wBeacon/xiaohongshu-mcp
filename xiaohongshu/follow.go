package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/sirupsen/logrus"
)

// FollowResult 关注/取消关注操作结果
type FollowResult struct {
	UserID  string `json:"user_id"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// 关注按钮选择器
const (
	// 用户主页关注按钮
	SelectorFollowButton = ".user-info .info-part .follow-button button"
)

// FollowAction 关注/取消关注操作
type FollowAction struct {
	page *rod.Page
}

// NewFollowAction 创建关注操作实例
func NewFollowAction(page *rod.Page) *FollowAction {
	return &FollowAction{page: page}
}

// Follow 关注指定用户，如果已关注则直接返回
func (a *FollowAction) Follow(ctx context.Context, userID, xsecToken string) (*FollowResult, error) {
	return a.perform(ctx, userID, xsecToken, true)
}

// Unfollow 取消关注指定用户，如果未关注则直接返回
func (a *FollowAction) Unfollow(ctx context.Context, userID, xsecToken string) (*FollowResult, error) {
	return a.perform(ctx, userID, xsecToken, false)
}

// perform 执行关注/取消关注操作
func (a *FollowAction) perform(ctx context.Context, userID, xsecToken string, targetFollow bool) (*FollowResult, error) {
	actionName := "关注"
	if !targetFollow {
		actionName = "取消关注"
	}

	page := a.page.Context(ctx).Timeout(60 * time.Second)

	// 导航到用户主页
	url := makeUserProfileURL(userID, xsecToken)
	logrus.Infof("打开用户主页执行%s: %s", actionName, url)
	page.MustNavigate(url)
	page.MustWaitDOMStable()
	time.Sleep(1 * time.Second)

	// 获取当前关注状态
	followed, err := a.getFollowState(page, userID)
	if err != nil {
		logrus.Warnf("获取关注状态失败: %v, 继续尝试点击", err)
		return a.clickFollowButton(page, userID, targetFollow, actionName)
	}

	// 已经是目标状态，直接返回
	if targetFollow && followed {
		logrus.Infof("用户 %s 已关注，跳过操作", userID)
		return &FollowResult{UserID: userID, Success: true, Message: "已关注"}, nil
	}
	if !targetFollow && !followed {
		logrus.Infof("用户 %s 未关注，跳过操作", userID)
		return &FollowResult{UserID: userID, Success: true, Message: "未关注"}, nil
	}

	return a.clickFollowButton(page, userID, targetFollow, actionName)
}

// clickFollowButton 点击关注/取消关注按钮
func (a *FollowAction) clickFollowButton(page *rod.Page, userID string, targetFollow bool, actionName string) (*FollowResult, error) {
	// 查找并点击关注按钮
	btn, err := page.Element(SelectorFollowButton)
	if err != nil {
		return nil, fmt.Errorf("未找到关注按钮: %w", err)
	}

	btn.MustClick()
	time.Sleep(2 * time.Second)

	// 验证操作结果
	followed, err := a.getFollowState(page, userID)
	if err != nil {
		logrus.Warnf("验证%s状态失败: %v", actionName, err)
		return &FollowResult{UserID: userID, Success: true, Message: actionName + "已执行(状态未验证)"}, nil
	}

	if followed == targetFollow {
		logrus.Infof("用户 %s %s成功", userID, actionName)
		return &FollowResult{UserID: userID, Success: true, Message: actionName + "成功"}, nil
	}

	// 第一次未成功，尝试再次点击
	logrus.Warnf("用户 %s %s可能未成功，尝试再次点击", userID, actionName)
	btn, err = page.Element(SelectorFollowButton)
	if err != nil {
		return nil, fmt.Errorf("第二次未找到关注按钮: %w", err)
	}

	btn.MustClick()
	time.Sleep(2 * time.Second)

	followed, err = a.getFollowState(page, userID)
	if err != nil {
		logrus.Warnf("第二次验证%s状态失败: %v", actionName, err)
		return &FollowResult{UserID: userID, Success: true, Message: actionName + "已执行(状态未验证)"}, nil
	}

	if followed == targetFollow {
		logrus.Infof("用户 %s 第二次%s成功", userID, actionName)
		return &FollowResult{UserID: userID, Success: true, Message: actionName + "成功"}, nil
	}

	return &FollowResult{UserID: userID, Success: false, Message: actionName + "可能未成功"}, nil
}

// getFollowState 从 __INITIAL_STATE__ 获取用户的关注状态
func (a *FollowAction) getFollowState(page *rod.Page, userID string) (bool, error) {
	result := page.MustEval(`() => {
		if (window.__INITIAL_STATE__ &&
		    window.__INITIAL_STATE__.user &&
		    window.__INITIAL_STATE__.user.userPageData) {
			const userPageData = window.__INITIAL_STATE__.user.userPageData;
			const data = userPageData.value !== undefined ? userPageData.value : userPageData._value;
			if (data) {
				return JSON.stringify(data);
			}
		}
		return "";
	}`).String()

	if result == "" {
		return false, fmt.Errorf("userPageData 未找到")
	}

	// 解析关注状态
	var userData struct {
		Interactions []struct {
			Type  string `json:"type"`
			Count string `json:"count"`
		} `json:"interactions"`
		BasicInfo struct {
			Followed bool `json:"followed"`
		} `json:"basicInfo"`
	}
	if err := json.Unmarshal([]byte(result), &userData); err != nil {
		return false, fmt.Errorf("解析 userPageData 失败: %w", err)
	}

	return userData.BasicInfo.Followed, nil
}
