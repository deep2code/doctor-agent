package knowledge

import (
	"context"
	"testing"
)

// TestRetrieverPopSupplement6DRecall guards the 2026-09-26 科普补充第六批(方向D):
// 儿童症状处理与意外伤害 (退烧药剂量与喂法、热性惊厥、儿童咳嗽用药、腹泻补液补锌与就医指征、
// 烫伤处理与预防、儿童中暑与心肺复苏、误服药与催吐、药品收纳、玩具与召回、跌倒、触电、家庭清查、
// 气道异物与海姆立克、溺水、纽扣电池、婴儿安全睡眠、狂犬病暴露与疫苗、狗咬预防、安全座椅、
// 手足口、WHO 腹泻脱水判断、儿童哮喘家庭管理).
// Condition names are topical phrases, so the gate asserts plain colloquial
// phrasing reaches top5 (same convention as directions A, B and C).
func TestRetrieverPopSupplement6DRecall(t *testing.T) {
	r := NewRetriever(newTestStore(t))
	cases := map[string]string{
		// 儿童退烧药：月龄、剂量与间隔
		"儿童退烧药什么时候该吃": "cs-antipyretic-dose",

		"宝宝几个月能用布洛芬":        "cs-antipyretic-dose",
		"对乙酰氨基酚要几个月大才能吃":    "cs-antipyretic-dose",
		"退烧药一次按体重吃多少":       "cs-antipyretic-dose",
		"布洛芬一次最多多少毫克":       "cs-antipyretic-dose",
		"对乙酰氨基酚一天最多能用多少":    "cs-antipyretic-dose",
		"退烧药隔几个小时能再吃一次":     "cs-antipyretic-dose",
		"蚕豆病能用对乙酰氨基酚吗":      "cs-antipyretic-dose",
		"孩子拉肚子脱水能吃布洛芬吗":     "cs-antipyretic-dose",
		"六个月的宝宝两种退烧药都能用吗":   "cs-antipyretic-dose",
		"成人退烧药能掰给十二岁以下的孩子吗": "cs-antipyretic-dose",
		"退烧药是按体重算还是按年龄算":    "cs-antipyretic-dose",
		"儿童退烧药有哪几种":         "cs-antipyretic-dose",

		// 退烧药怎么喂：交替、补喂、呕吐与复方感冒药
		"两种退烧药能不能换着吃":        "cs-antipyretic-rules",
		"退烧药和复方感冒药同服算不算重复用药": "cs-antipyretic-rules",

		"刚喂完退烧药就吐了要补吗":    "cs-antipyretic-rules",
		"退烧药喂完多久能再喂":      "cs-antipyretic-rules",
		"孩子睡着了发烧要不要叫醒喂药":  "cs-antipyretic-rules",
		"孩子发烧多久量一次体温":     "cs-antipyretic-rules",
		"吃了退烧药退不下来怎么办":    "cs-antipyretic-rules",
		"退下来又烧回去正常吗":      "cs-antipyretic-rules",
		"退热栓和口服退烧药哪个好":    "cs-antipyretic-rules",
		"两岁以下能不能吃复方感冒药":   "cs-antipyretic-rules",
		"孩子发烧要不要把退烧药吃勤一点": "cs-antipyretic-rules",

		// 热性惊厥：家庭急救与就医界限
		"孩子高热惊厥抽风了在家怎么处理": "cs-febrile-seizure-firstaid",

		"热性惊厥在家怎么急救":      "cs-febrile-seizure-firstaid",
		"小孩抽搐了要不要掐人中":     "cs-febrile-seizure-firstaid",
		"孩子抽搐时怎么抱":        "cs-febrile-seizure-firstaid",
		"抽搐超过几分钟必须就医":     "cs-febrile-seizure-firstaid",
		"热性惊厥一般抽多久会停":     "cs-febrile-seizure-firstaid",
		"多大的孩子会热性惊厥":      "cs-febrile-seizure-firstaid",
		"多少度发烧引起的抽搐算热性惊厥": "cs-febrile-seizure-firstaid",
		"烧了三天才抽要紧吗":       "cs-febrile-seizure-firstaid",
		"一天抽了好几次要紧吗":      "cs-febrile-seizure-firstaid",
		"抽过一次必须做脑电图吗":     "cs-febrile-seizure-firstaid",
		"什么叫做惊厥持续状态":      "cs-febrile-seizure-firstaid",
		"孩子没觉得发烧就抽了是癫痫吗":  "cs-febrile-seizure-firstaid",

		// 热性惊厥复发、遗传与疫苗
		"热性惊厥会复发吗":        "cs-febrile-seizure-recurrence",
		"抽过一次还会再抽吗":       "cs-febrile-seizure-recurrence",
		"热性惊厥复发率有多高":      "cs-febrile-seizure-recurrence",
		"什么样的孩子容易再抽":      "cs-febrile-seizure-recurrence",
		"大人小时候抽过会遗传给孩子吗":  "cs-febrile-seizure-recurrence",
		"热性惊厥会变成癫痫吗":      "cs-febrile-seizure-recurrence",
		"热性惊厥的孩子能打疫苗吗":    "cs-febrile-seizure-recurrence",
		"怕打疫苗发烧诱发抽搐还要不要打": "cs-febrile-seizure-recurrence",
		"是不是越小发作越爱复发":     "cs-febrile-seizure-recurrence",
		"什么情况要长期吃抗癫痫药预防":  "cs-febrile-seizure-recurrence",

		// 儿童咳嗽用药：消炎药、化痰药、雾化与慢性咳嗽
		"孩子咳嗽要不要吃消炎药":    "cs-child-cough-meds",
		"咳得厉害能不能给孩子喝止咳水": "cs-child-cough-meds",
		"孩子有痰要不要吃化痰药":    "cs-child-cough-meds",
		"化痰药几岁以下不能用":     "cs-child-cough-meds",
		"咳嗽就做雾化舒张支气管对吗":  "cs-child-cough-meds",
		"孩子咳嗽超过四周算慢性咳嗽吗": "cs-child-cough-meds",

		"干咳和有痰的治法一样吗":   "cs-child-cough-meds",
		"夜里干咳是咳嗽变异性哮喘吗": "cs-child-cough-meds",
		"家里有人抽烟孩子总咳怎么办": "cs-child-cough-meds",
		"咳嗽老不好是不是没治对":   "cs-child-cough-meds",
		"儿童咳嗽要不要用抗生素":   "cs-child-cough-meds",

		// 儿童腹泻：口服补液盐与补锌
		"口服补液盐怎么给孩子喝": "cs-diarrhea-ORS-zinc",
		"口服补液盐按体重怎么算": "cs-diarrhea-ORS-zinc",
		"孩子拉肚子怎么预防脱水": "cs-diarrhea-ORS-zinc",
		"每次拉完要补多少液体":  "cs-diarrhea-ORS-zinc",
		"低渗口服补液盐好在哪":  "cs-diarrhea-ORS-zinc",

		"儿童腹泻补锌补多少毫克":  "cs-diarrhea-ORS-zinc",
		"补锌要吃几天":       "cs-diarrhea-ORS-zinc",
		"六个月以上的孩子补锌剂量": "cs-diarrhea-ORS-zinc",

		"宝宝腹泻期间能不能正常吃饭": "cs-diarrhea-ORS-zinc",
		"拉肚子要不要换免乳糖奶粉":  "cs-diarrhea-ORS-zinc",
		"腹泻多长时间算慢性腹泻":   "cs-diarrhea-ORS-zinc",

		// 儿童腹泻就医指征与脱水识别
		"孩子拉肚子哪些情况要去医院": "cs-diarrhea-danger-signs",
		"宝宝腹泻多久必须送医院":   "cs-diarrhea-danger-signs",

		"三个月的宝宝发烧要去医院吗": "cs-diarrhea-danger-signs",
		"拉肚子呕吐喝不进去药怎么办": "cs-diarrhea-danger-signs",
		"孩子大便带血要紧吗":     "cs-diarrhea-danger-signs",
		"怎么看出孩子脱水了":     "cs-diarrhea-danger-signs",
		"腹泻眼窝凹陷泪少怎么办":   "cs-diarrhea-danger-signs",
		"喝了补液盐还是脱水怎么办":  "cs-diarrhea-danger-signs",
		"口服补液失败有哪些信号":   "cs-diarrhea-danger-signs",
		"孩子腹泻什么情况能在家看":  "cs-diarrhea-danger-signs",
		"水样便腹泻该不该用抗生素":  "cs-diarrhea-danger-signs",

		// 烫伤处理辟谣：牙膏酱油、冰敷、脱衣与水疱
		"为什么不能给烫伤处涂牙膏": "cs-burn-no-toothpaste",
		"孩子烫伤能不能抹牙膏":   "cs-burn-no-toothpaste",
		"宝宝烫伤涂牙膏对不对":   "cs-burn-no-toothpaste",

		"孩子烫伤能涂香油吗":         "cs-burn-no-toothpaste",
		"烫伤能抹紫药水红药水吗":       "cs-burn-no-toothpaste",
		"抹了牙膏去医院会更难处理吗":     "cs-burn-no-toothpaste",
		"烫伤能直接冰敷吗":          "cs-burn-no-toothpaste",
		"烫伤后能不能抹药膏":         "cs-burn-no-toothpaste",
		"烫伤冲水时水流要不要对着伤口":    "cs-burn-no-toothpaste",
		"脸上烫伤怎么冷敷":          "cs-burn-no-toothpaste",
		"烫伤后能不能用冰水和冰块":      "cs-burn-no-toothpaste",
		"烫伤面积大要一边冲水一边打120吗": "cs-burn-no-toothpaste",
		"衣物粘在烫伤皮肤上能硬拽吗":     "cs-burn-no-toothpaste",
		"孩子烫伤会不会留疤和残疾":      "cs-burn-no-toothpaste",
		"烧伤烫伤水泡别自己挑":        "cs-burn-no-toothpaste",

		// 儿童中暑与车内滞留
		"儿童中暑头晕怎么处理":    "cs-heat-stroke-child",
		"中暑先把腿抬高还是头抬高":  "cs-heat-stroke-child",
		"中暑降温敷在哪些部位":    "cs-heat-stroke-child",
		"中暑给孩子喝淡盐水怎么配":  "cs-heat-stroke-child",
		"中暑能喝冰水吗":       "cs-heat-stroke-child",
		"中暑抽搐要不要往嘴里塞东西": "cs-heat-stroke-child",
		"中暑抽搐还能喂水吗":     "cs-heat-stroke-child",
		"夏天几点别带孩子出门":    "cs-heat-stroke-child",
		"孩子被锁在车里会热射病吗":  "cs-heat-stroke-child",

		// 儿童心肺复苏与外伤处置
		"儿童心肺复苏按压位置":       "cs-child-cpr-and-trauma",
		"给孩子做心肺复苏一分钟按多少次":  "cs-child-cpr-and-trauma",
		"儿童胸外按压按多深":        "cs-child-cpr-and-trauma",
		"孩子溺水捞上来怎么判断有没有呼吸": "cs-child-cpr-and-trauma",
		"伤口里扎了玻璃能自己拔吗":     "cs-child-cpr-and-trauma",
		"摔到脖子能不能抱孩子":       "cs-child-cpr-and-trauma",
		"怀疑颈椎受伤怎么固定":       "cs-child-cpr-and-trauma",

		// 孩子误服药：第一步做什么
		"孩子偷吃了大人的药怎么办":   "cs-mis-ingestion-first-step",
		"宝宝把药当糖吃了怎么办":    "cs-mis-ingestion-first-step",
		"发现孩子误吃药第一件事做什么": "cs-mis-ingestion-first-step",
		"误服超过半小时还能在家处理吗": "cs-mis-ingestion-first-step",
		"孩子误吃药要不要马上洗胃":   "cs-mis-ingestion-first-step",
		"孩子误吃降压药要紧吗":     "cs-mis-ingestion-first-step",
		"宝宝吃了降糖药怎么办":     "cs-mis-ingestion-first-step",
		"误吃精神类药物怎么办":     "cs-mis-ingestion-first-step",
		"去医院要不要带上药盒":     "cs-mis-ingestion-first-step",
		"不知道孩子吃了什么药怎么办":  "cs-mis-ingestion-first-step",
		"要不要把呕吐物带去医院":    "cs-mis-ingestion-first-step",
		"孩子误吃药能吓唬他吗":     "cs-mis-ingestion-first-step",

		// 误服药催吐：怎么做与哪些情况不能做
		"孩子误吃药怎么催吐":    "cs-mis-ingestion-vomit",
		"催吐是先喝水还是先抠喉咙": "cs-mis-ingestion-vomit",
		"怎么知道孩子催吐成功了":  "cs-mis-ingestion-vomit",
		"哪些情况不能给孩子催吐":  "cs-mis-ingestion-vomit",
		"孩子抽搐还能催吐吗":    "cs-mis-ingestion-vomit",
		"误服洁厕灵能催吐吗":    "cs-mis-ingestion-vomit",
		"孩子睡着了能催吐吗":    "cs-mis-ingestion-vomit",
		"催吐用温水还是自来水":   "cs-mis-ingestion-vomit",
		"孩子空腹还能催吐吗":    "cs-mis-ingestion-vomit",
		"去医院路上能不能先催吐":  "cs-mis-ingestion-vomit",
		"催吐和洗胃哪个更彻底":   "cs-mis-ingestion-vomit",

		// 家庭药品收纳与防误服
		"家里药品怎么收纳":       "cs-drug-storage-childproof",
		"药要放在孩子拿不到的地方":   "cs-drug-storage-childproof",
		"外用药能装在饮料瓶里吗":    "cs-drug-storage-childproof",
		"消毒液能和药放一起吗":     "cs-drug-storage-childproof",
		"哄孩子吃药能说这是糖吗":    "cs-drug-storage-childproof",
		"大人的药能给孩子吃吗":     "cs-drug-storage-childproof",
		"怎么教孩子认识药的危险":    "cs-drug-storage-childproof",
		"家里有人吃抗抑郁药要注意什么": "cs-drug-storage-childproof",
		"抽屉里的药要锁起来吗":     "cs-drug-storage-childproof",
		"洗衣液消毒液放哪里安全":    "cs-drug-storage-childproof",
		"家长要学哪些中毒急救":     "cs-drug-storage-childproof",

		// 儿童玩具怎么选：3C 认证与标签
		"儿童玩具怎么选":        "cs-toy-buying-safety",
		"给孩子买玩具要看哪些标识":   "cs-toy-buying-safety",
		"哪些儿童用品必须有3C认证":  "cs-toy-buying-safety",
		"没有CCC标志的玩具能买吗":  "cs-toy-buying-safety",
		"怎么查玩具在不在3C目录里":  "cs-toy-buying-safety",
		"海淘玩具没有中文标签正常吗":  "cs-toy-buying-safety",
		"进口产品要有中文标签吗":    "cs-toy-buying-safety",
		"三岁以下能玩带小零件的玩具吗": "cs-toy-buying-safety",
		"玩具上的警示语要看吗":     "cs-toy-buying-safety",
		"玩具标注的适用年龄是硬要求吗": "cs-toy-buying-safety",
		"地摊上的三无玩具能买吗":    "cs-toy-buying-safety",
		"玩具有保质期或使用年限吗":   "cs-toy-buying-safety",
		"买玩具要留什么凭证":      "cs-toy-buying-safety",
		"玩具割伤手算质量问题吗":    "cs-toy-buying-safety",

		// 产品召回与使用说明
		"怎么查我家玩具有没有被召回":  "cs-product-use-recall",
		"产品被召回了找谁":       "cs-product-use-recall",
		"召回公告在哪里查":       "cs-product-use-recall",
		"玩具电器坏了能自己修吗":    "cs-product-use-recall",
		"过期的儿童用品还能用吗":    "cs-product-use-recall",
		"安全使用年限写在哪里":     "cs-product-use-recall",
		"孩子用滑板车要戴护具吗":    "cs-product-use-recall",
		"儿童用品的使用说明要看吗":   "cs-product-use-recall",
		"发现玩具伤过孩子向谁举报":   "cs-product-use-recall",
		"封闭房间用燃气炉为什么会中毒": "cs-product-use-recall",

		// 儿童跌倒预防
		"儿童跌倒预防要注意什么":  "cs-child-fall-prevention",
		"孩子摔一跤要紧吗":     "cs-child-fall-prevention",
		"跌倒能摔出什么后果":    "cs-child-fall-prevention",
		"怎么让孩子不爬窗台阳台":  "cs-child-fall-prevention",
		"楼梯上追逐有多危险":    "cs-child-fall-prevention",
		"小孩穿什么鞋不容易摔跤":  "cs-child-fall-prevention",
		"裤子太长会让孩子摔倒吗":  "cs-child-fall-prevention",
		"学轮滑要买哪些护具":    "cs-child-fall-prevention",
		"孩子运动前要不要热身":   "cs-child-fall-prevention",
		"小区健身器材安全吗":    "cs-child-fall-prevention",
		"拖完地孩子能跑吗":     "cs-child-fall-prevention",
		"家里的门槛台阶要注意什么": "cs-child-fall-prevention",

		// 儿童触电：施救断挑呼与家庭用电规矩
		"孩子触电了能直接用手拉吗": "cs-electric-shock-child",
		"触电第一步关什么":     "cs-electric-shock-child",
		"找不到电闸怎么把电线挑开": "cs-electric-shock-child",
		"能用扫帚挑开电线吗":    "cs-electric-shock-child",
		"触电后没呼吸怎么办":    "cs-electric-shock-child",
		"孩子被电了一下手麻要紧吗": "cs-electric-shock-child",
		"触电当时没事要不要去医院": "cs-electric-shock-child",
		"为什么抓住电线松不开":   "cs-electric-shock-child",
		"触电伤口看着小要紧吗":   "cs-electric-shock-child",
		"湿手能不能碰开关":     "cs-electric-shock-child",
		"孩子拿筷子捅插座孔怎么办": "cs-electric-shock-child",
		"边充电边玩平板有危险吗":  "cs-electric-shock-child",
		"地上掉着电线怎么绕开":   "cs-electric-shock-child",
		"插座要不要装防水盖":    "cs-electric-shock-child",
		"雷雨天能不能洗澡":     "cs-electric-shock-child",

		// 家庭安全清查
		"家庭安全清查要做哪些":    "cs-safe-home-checklist",
		"家里哪些地方容易让孩子出事": "cs-safe-home-checklist",
		"多久查一次家里的安全隐患":  "cs-safe-home-checklist",
		"能让孩子一起找安全隐患吗":  "cs-safe-home-checklist",
		"桌角护角要不要买":      "cs-safe-home-checklist",
		"窗台阳台要怎么防坠落":    "cs-safe-home-checklist",
		"暖气热水瓶怎么防护":     "cs-safe-home-checklist",
		"社区有预防伤害的培训吗":   "cs-safe-home-checklist",
		"孩子摔伤后要记录什么":    "cs-safe-home-checklist",
		"带孩子去哪体验安全教育":   "cs-safe-home-checklist",
		"儿童安全锁要不要买":     "cs-safe-home-checklist",

		// 儿童窒息预防
		"怎么防孩子被小东西噎住":   "cs-choking-prevention",
		"孩子吃饭爱跑爱笑容易噎着吗": "cs-choking-prevention",

		"玩具掉扣子还能给孩子玩吗":      "cs-choking-prevention",
		"含鱼刺小块骨头的食物能不能给孩子吃": "cs-choking-prevention",
		"小磁铁玩具几岁能玩":         "cs-choking-prevention",
		"储物间杂物间要防着孩子进吗":     "cs-choking-prevention",
		"家里危险品怎么收纳":         "cs-choking-prevention",
		"窒息是儿童常见死因吗":        "cs-choking-prevention",

		// 气道异物梗阻识别与就医
		"噎住的急救黄金时间是几分钟":  "cs-choking-recognize",
		"孩子噎住还能说话要不要急救":  "cs-choking-recognize",
		"双手抓住脖子说不出话":     "cs-choking-recognize",
		"先打120还是先做海姆立克":  "cs-choking-recognize",
		"异物咳出来了还要去医院吗":   "cs-choking-recognize",
		"海姆立克会不会有并发症":    "cs-choking-recognize",
		"孩子吸进东西先侧身还是先拍背": "cs-choking-recognize",
		"口唇紫绀呼吸停止怎么办":    "cs-choking-recognize",
		"海姆立克能拿真人练习吗":    "cs-choking-recognize",

		// 海姆立克具体手法：成人孕妇肥胖婴儿自救
		"海姆立克急救法怎么做":    "cs-heimlich-technique",
		"海姆立克手放在哪里":     "cs-heimlich-technique",
		"海姆立克一次冲击几下":    "cs-heimlich-technique",
		"孕妇噎住了能不能压肚子":   "cs-heimlich-technique",
		"肥胖的人被噎住怎么救":    "cs-heimlich-technique",
		"一岁以下婴儿海姆立克拍背法": "cs-heimlich-technique",
		"婴儿胸部按压用几根手指":   "cs-heimlich-technique",
		"拍背没用下一步怎么做":    "cs-heimlich-technique",
		"一个人在家被噎住怎么自救":  "cs-heimlich-technique",
		"婴儿拍着拍着没反应了怎么办": "cs-heimlich-technique",

		// 儿童溺水预防

		"家里有泳池要装护栏吗":     "cs-drowning-prevention",
		"水井水缸要不要加盖":      "cs-drowning-prevention",
		"教会游泳就不会溺水吗":     "cs-drowning-prevention",
		"报游泳班要看哪些安全条件":   "cs-drowning-prevention",
		"学龄前儿童怎么监督照护防溺水": "cs-drowning-prevention",
		"洪灾死者有多少是溺亡":     "cs-drowning-prevention",

		// 纽扣电池误吞的危害与时间窗
		"误吞纽扣电池的危害":      "cs-button-battery-harm",
		"纽扣电池卡在食道多久开始烧伤": "cs-button-battery-harm",
		"误吞电池超过6小时会穿孔吗":  "cs-button-battery-harm",
		"电池15分钟就能灼伤食道吗":  "cs-button-battery-harm",
		"纽扣电池为什么像电烙铁":    "cs-button-battery-harm",
		"误吞纽扣电池会不会死":     "cs-button-battery-harm",
		"电池塞进鼻子里会毁容吗":    "cs-button-battery-harm",
		"电池取出后食道狭窄怎么办":   "cs-button-battery-harm",
		"误吞电池会留下后遗症吗":    "cs-button-battery-harm",
		"小孩误吞电池的多不多":     "cs-button-battery-harm",

		// 纽扣电池误吞急救
		"误吞纽扣电池急救怎么办": "cs-button-battery-firstaid",
		"误吞电池能不能催吐":   "cs-button-battery-firstaid",

		"纽扣电池能胃镜取出来吗":   "cs-button-battery-firstaid",
		"误吞电池到医院拍什么片":   "cs-button-battery-firstaid",
		"吞了硬币能自己排出来吗":   "cs-button-battery-firstaid",
		"电池取出后还要复查吗":    "cs-button-battery-firstaid",
		"看病要跟医生说电池什么信息": "cs-button-battery-firstaid",

		// 消化道异物误吞信号与收纳
		"孩子误吞电池后会有什么反常表现": "cs-button-battery-signs",

		"宝宝呕吐物中混有黑色液体":  "cs-button-battery-signs",
		"小宝宝莫名哭闹捂着肚子":   "cs-button-battery-signs",
		"又低热又咳嗽会不会卡了东西": "cs-button-battery-signs",
		"鼻子一侧流脓是不是塞了东西": "cs-button-battery-signs",
		"孩子拒食流口水要警惕吗":   "cs-button-battery-signs",
		"没看见孩子吞东西会不会耽误": "cs-button-battery-signs",
		"家里的电池硬币怎么收纳":   "cs-button-battery-signs",
		"玩具电池盖不牢能用胶带粘吗": "cs-button-battery-signs",

		// 婴儿安全睡眠与摇篮死亡
		"什么是摇篮死亡":       "cs-infant-safe-sleep",
		"摇篮死亡的概率有多大":    "cs-infant-safe-sleep",
		"宝宝多大月龄最容易猝死":   "cs-infant-safe-sleep",
		"新生儿该趴睡还是仰睡":    "cs-infant-safe-sleep",
		"婴儿床能不能放毛绒玩具":   "cs-infant-safe-sleep",
		"要不要跟宝宝同床睡":     "cs-infant-safe-sleep",
		"能在安全座椅里睡一整觉吗":  "cs-infant-safe-sleep",
		"婴儿房温度湿度多少合适":   "cs-infant-safe-sleep",
		"给宝宝盖被子怎么防蒙住口鼻": "cs-infant-safe-sleep",
		"喝完奶要不要拍嗝":      "cs-infant-safe-sleep",
		"婴儿安全睡眠要注意什么":   "cs-infant-safe-sleep",

		// 狂犬病暴露分级与伤口处置
		"狂犬病暴露分级怎么分":        "cs-rabies-exposure-grades",
		"被狗咬了算几级暴露":         "cs-rabies-exposure-grades",
		"皮肤完好被狗舔了要打针吗":      "cs-rabies-exposure-grades",
		"咬伤没出血要不要接种狂犬疫苗":    "cs-rabies-exposure-grades",
		"咬出血属于三级暴露吗":        "cs-rabies-exposure-grades",
		"被猫抓破皮算几级暴露":        "cs-rabies-exposure-grades",
		"狂犬病暴露后伤口要用肥皂水冲多久":  "cs-rabies-exposure-grades",
		"伤口已经结痂还要处理吗":       "cs-rabies-exposure-grades",
		"咬伤能不能缝针":           "cs-rabies-exposure-grades",
		"狂犬疫苗和破伤风能不能同一只胳膊打": "cs-rabies-exposure-grades",
		"伤到眼睛或嘴巴的伤口怎么冲洗":    "cs-rabies-exposure-grades",

		// 狂犬病疫苗免疫程序
		"狂犬病疫苗免疫程序是什么":       "cs-rabies-vaccine-schedule",
		"狂犬疫苗五针法哪天打":         "cs-rabies-vaccine-schedule",
		"孩子打狂犬疫苗打在哪个部位":      "cs-rabies-vaccine-schedule",
		"两岁以下的孩子狂犬疫苗打大腿还是胳膊": "cs-rabies-vaccine-schedule",

		"小孩打狂犬疫苗按体重减量吗":    "cs-rabies-vaccine-schedule",
		"狂犬疫苗有一针晚了要不要重打":   "cs-rabies-vaccine-schedule",
		"五针法打到一半能换成另一种疫苗吗": "cs-rabies-vaccine-schedule",

		"被咬几个月后还能补打狂犬疫苗吗": "cs-rabies-vaccine-schedule",
		"打狂犬疫苗期间能接种其他疫苗吗": "cs-rabies-vaccine-schedule",
		"孩子发烧能不能打狂犬疫苗":    "cs-rabies-vaccine-schedule",
		"狂犬疫苗四针法第几天打":     "cs-rabies-vaccine-schedule",

		// 狂犬病被动免疫制剂
		"狂犬病免疫球蛋白按体重多少":    "cs-rabies-passive-immunization",
		"抗狂犬病血清每千克多少国际单位":  "cs-rabies-passive-immunization",
		"免疫球蛋白为什么打在伤口周围":   "cs-rabies-passive-immunization",
		"咬到手指打免疫球蛋白怎么控制剂量": "cs-rabies-passive-immunization",
		"当天没打上免疫球蛋白几天内还能补": "cs-rabies-passive-immunization",
		"狂犬疫苗打完要不要查抗体":     "cs-rabies-passive-immunization",
		"以前打过狂犬疫苗又被咬了怎么办":  "cs-rabies-passive-immunization",
		"全程接种后隔半年再被咬打几针":   "cs-rabies-passive-immunization",
		"家养孩子能不能提前打狂犬疫苗":   "cs-rabies-passive-immunization",
		"抗狂犬病血清注射前要做过敏试验吗": "cs-rabies-passive-immunization",

		// 预防儿童被狗咬伤的行为教育
		"怎么教孩子不被狗咬":    "cs-dog-bite-behavior",
		"遇见别人的狗能直接摸吗":  "cs-dog-bite-behavior",
		"孩子盯着狗看会不会被咬":  "cs-dog-bite-behavior",
		"被狗追时大声尖叫有用吗":  "cs-dog-bite-behavior",
		"狗吃饭睡觉时能不能摸":   "cs-dog-bite-behavior",
		"从背后拍狗会被咬吗":    "cs-dog-bite-behavior",
		"小区里的流浪狗能喂吗":   "cs-dog-bite-behavior",
		"孩子被狗咬了不敢说怎么办": "cs-dog-bite-behavior",
		"先让狗闻一闻再摸对吗":   "cs-dog-bite-behavior",

		// 儿童烫伤预防与偏方辟谣
		"儿童烫伤预防五防是什么":     "cs-burn-home-prevention",
		"烧水壶电暖炉放哪儿":       "cs-burn-home-prevention",
		"给孩子放洗澡水先放冷水还是热水": "cs-burn-home-prevention",
		"做饭时孩子能不能待在厨房":    "cs-burn-home-prevention",
		"卷发棒熨斗放桌上危险吗":     "cs-burn-home-prevention",
		"放烟花爆竹怎么防孩子烧伤":    "cs-burn-home-prevention",
		"烫伤抹麦菜水茶油行不行":     "cs-burn-home-prevention",
		"烫伤涂酱油醋会不会影响医生判断": "cs-burn-home-prevention",
		"中国每年多少儿童烧烫伤":     "cs-burn-home-prevention",
		"防烫伤挡板隔热手套有什么用":   "cs-burn-home-prevention",

		// 儿童安全座椅分阶段
		"新生儿要不要用安全座椅":        "cs-car-seat-stages",
		"婴儿提篮算不算安全座椅":        "cs-car-seat-stages",
		"安全座椅反向坐到多大":         "cs-car-seat-stages",
		"为什么婴儿座椅要朝后装":        "cs-car-seat-stages",
		"多大可以正向坐安全座椅":        "cs-car-seat-stages",
		"增高垫几岁开始用":           "cs-car-seat-stages",
		"体重多少公斤要换增高垫":        "cs-car-seat-stages",
		"孩子多大可以不坐安全座椅系成人安全带": "cs-car-seat-stages",
		"身高145厘米是什么门槛":       "cs-car-seat-stages",
		"儿童安全座椅能起到什么作用":      "cs-car-seat-stages",

		// 儿童安全座椅选购与使用
		"安全座椅要有CCC标志吗":    "cs-car-seat-buying-use",
		"怎么查安全座椅3C证书真假":   "cs-car-seat-buying-use",
		"ISOFIX和安全带固定选哪个": "cs-car-seat-buying-use",
		"安全座椅织带多宽才合格":     "cs-car-seat-buying-use",
		"儿童安全坐垫能替代安全座椅吗":  "cs-car-seat-buying-use",
		"装完怎么检查座椅牢不牢":     "cs-car-seat-buying-use",
		"安全带留一根手指的松紧对吗":   "cs-car-seat-buying-use",
		"孩子衣服绳带围巾坐车要不要处理": "cs-car-seat-buying-use",
		"车内儿童锁在哪儿开":       "cs-car-seat-buying-use",
		"能不能抱着孩子坐副驾驶":     "cs-car-seat-buying-use",
		"出过车祸的安全座椅还能用吗":   "cs-car-seat-buying-use",

		// 手足口病表现与病程
		"手足口病潜伏期是几天":     "cs-hfmd-recognition",
		"接触过手足口的孩子多久会发病": "cs-hfmd-recognition",
		"手足口病的疹子痒不痒":     "cs-hfmd-recognition",
		"手足口皮疹会不会留疤":     "cs-hfmd-recognition",
		"只有嘴里起泡是手足口吗":    "cs-hfmd-recognition",
		"手足口病指甲脱落是怎么回事":  "cs-hfmd-recognition",
		"手足口病几岁的孩子容易得":   "cs-hfmd-recognition",
		"手足口病皮疹会长在屁股上吗":  "cs-hfmd-recognition",
		"得过一次手足口还会再得吗":   "cs-hfmd-recognition",
		"手足口病程几天痊愈":      "cs-hfmd-recognition",

		// 手足口病重症早期识别与家庭护理
		"手足口病重症早期识别看哪些指标":   "cs-hfmd-redflags-care",
		"手足口哪类孩子容易变重症":      "cs-hfmd-redflags-care",
		"手足口持续高热不退危险吗":      "cs-hfmd-redflags-care",
		"手足口病安静时呼吸多少次算危险":   "cs-hfmd-redflags-care",
		"孩子心率超过160次每分要紧吗":   "cs-hfmd-redflags-care",
		"毛细血管再充盈时间超过2秒说明什么": "cs-hfmd-redflags-care",
		"手足口病白细胞多少提示重症":     "cs-hfmd-redflags-care",
		"手足口血糖8.3是什么意思":     "cs-hfmd-redflags-care",
		"手足口疫苗几个月能打":        "cs-hfmd-redflags-care",
		"手足口疫苗打几针间隔多久":      "cs-hfmd-redflags-care",
		"酒精能不能杀死手足口病毒":      "cs-hfmd-redflags-care",
		"手足口要不要用抗病毒药":       "cs-hfmd-redflags-care",

		// WHO 腹泻分类与家庭脱水判断
		"怎么判断是不是重度脱水": "cs-diarrhea-dehydration-who",

		"一天拉几次才算腹泻":      "cs-diarrhea-dehydration-who",
		"母乳宝宝糊状大便是腹泻吗":   "cs-diarrhea-dehydration-who",
		"拉肚子只喝白开水行不行":    "cs-diarrhea-dehydration-who",
		"口服补液盐和糖盐水有什么区别": "cs-diarrhea-dehydration-who",
		"怎么在家看孩子有没有脱水":   "cs-diarrhea-dehydration-who",
		"孩子眼窝凹陷是不是脱水":    "cs-diarrhea-dehydration-who",
		"捏起皮肤多久回弹算脱水":    "cs-diarrhea-dehydration-who",
		"宝宝烦躁口渴是不是脱水表现":  "cs-diarrhea-dehydration-who",

		"腹泻期间还要不要继续喂母乳": "cs-diarrhea-dehydration-who",

		"什么时候腹泻必须输液": "cs-diarrhea-dehydration-who",

		// 儿童哮喘家庭管理
		"儿童哮喘家庭管理要做哪几件事":   "cs-child-asthma-home",
		"一到夜里就咳是不是哮喘":      "cs-child-asthma-home",
		"孩子喘是不是哮喘":         "cs-child-asthma-home",
		"小孩哮喘能不能自愈":        "cs-child-asthma-home",
		"哮喘不咳嗽了能停药吗":       "cs-child-asthma-home",
		"父母有过敏孩子会遗传吗":      "cs-child-asthma-home",
		"家里养猫狗孩子哮喘要注意吗":    "cs-child-asthma-home",
		"孩子一跑步就喘还能运动吗":     "cs-child-asthma-home",
		"哮喘发作前有什么征兆":       "cs-child-asthma-home",
		"哮喘日记要记什么":         "cs-child-asthma-home",
		"哮喘孩子要避免哪些吸入过敏原":   "cs-child-asthma-home",
		"三岁孩子查不了肺功能怎么确诊哮喘": "cs-child-asthma-home",
	}

	ctx := context.Background()
	for query, want := range cases {
		res, err := r.Retrieve(ctx, query, 5)
		if err != nil {
			t.Fatalf("查询 %q 检索失败: %v", query, err)
		}
		found := false
		for _, item := range res {
			if item.Entry.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("查询 %q: 期望 top5 含 %s，实际 %v", query, want, entryIDs(res))
		}
	}
}
