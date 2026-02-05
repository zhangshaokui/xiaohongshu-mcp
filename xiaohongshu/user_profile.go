package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

type UserProfileAction struct {
	page *rod.Page
}

func NewUserProfileAction(page *rod.Page) *UserProfileAction {
	pp := page.Timeout(60 * time.Second)
	return &UserProfileAction{page: pp}
}

// UserProfile 获取用户基本信息及帖子
func (u *UserProfileAction) UserProfile(ctx context.Context, userID, xsecToken string) (*UserProfileResponse, error) {
	page := u.page.Context(ctx)

	searchURL := makeUserProfileURL(userID, xsecToken)
	page.MustNavigate(searchURL)
	page.MustWaitStable()

	return u.extractUserProfileData(page)
}

// extractUserProfileData 从页面中提取用户资料数据的通用方法
func (u *UserProfileAction) extractUserProfileData(page *rod.Page) (*UserProfileResponse, error) {
	page.MustWait(`() => window.__INITIAL_STATE__ !== undefined`)

	userDataResult := page.MustEval(`() => {
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

	if userDataResult == "" {
		return nil, fmt.Errorf("user.userPageData.value not found in __INITIAL_STATE__")
	}

	// 2. 获取用户帖子：window.__INITIAL_STATE__.user.notes.value
	notesResult := page.MustEval(`() => {
		if (window.__INITIAL_STATE__ &&
		    window.__INITIAL_STATE__.user &&
		    window.__INITIAL_STATE__.user.notes) {
			const notes = window.__INITIAL_STATE__.user.notes;
			// 优先使用 value（getter），如果不存在则使用 _value（内部字段）
			const data = notes.value !== undefined ? notes.value : notes._value;
			if (data) {
				return JSON.stringify(data);
			}
		}
		return "";
	}`).String()

	if notesResult == "" {
		return nil, fmt.Errorf("user.notes.value not found in __INITIAL_STATE__")
	}

	// 解析用户信息
	var userPageData struct {
		Interactions []UserInteractions `json:"interactions"`
		BasicInfo    UserBasicInfo      `json:"basicInfo"`
	}
	if err := json.Unmarshal([]byte(userDataResult), &userPageData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal userPageData: %w", err)
	}

	// 从 DOM 元素中提取准确的关注/粉丝/获赞数据
	// 反向搜索：从纯数字元素开始，查找附近的关键词
	domInteractions := page.MustEval(`() => {
		const result = [];

		// 查找所有只包含数字的元素
		const allElements = Array.from(document.querySelectorAll('*'));
		const numberElements = [];

		for (const elem of allElements) {
			const text = elem.textContent.trim();
			// 匹配纯数字或带万的数字，如 2253, 3.1万, 62.9万
			const match = text.match(/^([0-9]+(?:\\.[0-9]+)?[万]?)$/);
			if (match) {
				const num = match[1];
				// 跳过过长数字（可能是ID）
				if (num.length > 8) continue;
				// 跳过过小数字
				if (num === '0' || num === '1') continue;
				// 跳过包含"+"的模糊数字
				if (text.includes('+')) continue;
				numberElements.push({ elem, num });
			}
		}

		// 对每个数字元素，检查其父元素的子元素中是否有关键词
		const keywordMap = [
			{ keyword: '关注', type: 'follows', name: '关注' },
			{ keyword: '粉丝', type: 'fans', name: '粉丝' },
			{ keyword: '获赞', type: 'interaction', name: '获赞与收藏' }
		];

		for (const kw of keywordMap) {
			for (const { elem, num } of numberElements) {
				// 检查父元素的所有子元素是否包含关键词
				const parent = elem.parentElement;
				if (!parent) continue;

				const parentText = parent.textContent || '';
				if (!parentText.includes(kw.keyword)) continue;

				// 检查父元素的直接子元素
				for (const child of parent.children) {
					const childText = child.textContent || '';
					if (childText.includes(kw.keyword)) {
						result.push({ type: kw.type, name: kw.name, count: num });
						break;
					}
				}

				// 如果找到了就不再查找这个关键词
				if (result.find(r => r.type === kw.type)) break;
			}
		}

		return JSON.stringify(result);
	}`).String()

	fmt.Printf("[DEBUG] DOM 提取结果: %s\n", domInteractions)

	// 如果 DOM 中找到了准确数据，优先使用
	if domInteractions != "[]" {
		var domInteractionsList []UserInteractions
		if err := json.Unmarshal([]byte(domInteractions), &domInteractionsList); err == nil && len(domInteractionsList) > 0 {
			userPageData.Interactions = domInteractionsList
		}
	}

	// 解析帖子数据（帖子为双重数组）
	var notesFeeds [][]Feed
	if err := json.Unmarshal([]byte(notesResult), &notesFeeds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notes: %w", err)
	}

	// 组装响应
	response := &UserProfileResponse{
		UserBasicInfo: userPageData.BasicInfo,
		Interactions:  userPageData.Interactions,
	}

	// 添加用户帖子（展平双重数组）
	for _, feeds := range notesFeeds {
		if len(feeds) != 0 {
			response.Feeds = append(response.Feeds, feeds...)
		}
	}

	return response, nil
}

func makeUserProfileURL(userID, xsecToken string) string {
	return fmt.Sprintf("https://www.xiaohongshu.com/user/profile/%s?xsec_token=%s&xsec_source=pc_note", userID, xsecToken)
}

func (u *UserProfileAction) GetMyProfileViaSidebar(ctx context.Context) (*UserProfileResponse, error) {
	page := u.page.Context(ctx)

	// 创建导航动作
	navigate := NewNavigate(page)

	// 通过侧边栏导航到个人主页
	if err := navigate.ToProfilePage(ctx); err != nil {
		return nil, fmt.Errorf("failed to navigate to profile page via sidebar: %w", err)
	}

	// 等待页面加载完成并获取 __INITIAL_STATE__
	page.MustWaitStable()

	return u.extractUserProfileData(page)
}
