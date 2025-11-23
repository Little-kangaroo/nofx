package config

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nofx/market"
	"os"
	"slices"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database 配置数据库
type Database struct {
	db *sql.DB
}

// NewDatabase 创建配置数据库
func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	database := &Database{db: db}
	if err := database.createTables(); err != nil {
		return nil, fmt.Errorf("创建表失败: %w", err)
	}

	if err := database.initDefaultData(); err != nil {
		return nil, fmt.Errorf("初始化默认数据失败: %w", err)
	}

	// 验证数据库表结构
	if err := database.verifyTableStructure(); err != nil {
		log.Printf("⚠️ 数据库表结构验证失败: %v", err)
	}

	return database, nil
}

// createTables 创建数据库表
func (d *Database) createTables() error {
	queries := []string{
		// AI模型配置表
		`CREATE TABLE IF NOT EXISTS ai_models (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			enabled BOOLEAN DEFAULT 0,
			api_key TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,

		// 交易所配置表
		`CREATE TABLE IF NOT EXISTS exchanges (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			type TEXT NOT NULL, -- 'cex' or 'dex'
			enabled BOOLEAN DEFAULT 0,
			api_key TEXT DEFAULT '',
			secret_key TEXT DEFAULT '',
			testnet BOOLEAN DEFAULT 0,
			-- Hyperliquid 特定字段
			hyperliquid_wallet_addr TEXT DEFAULT '',
			-- Aster 特定字段
			aster_user TEXT DEFAULT '',
			aster_signer TEXT DEFAULT '',
			aster_private_key TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,

		// 用户信号源配置表
		`CREATE TABLE IF NOT EXISTS user_signal_sources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			coin_pool_url TEXT DEFAULT '',
			oi_top_url TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			UNIQUE(user_id)
		)`,

		// 交易员配置表
		`CREATE TABLE IF NOT EXISTS traders (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			ai_model_id TEXT NOT NULL,
			exchange_id TEXT NOT NULL,
			initial_balance REAL NOT NULL,
			scan_interval_minutes INTEGER DEFAULT 3,
			is_running BOOLEAN DEFAULT 0,
			btc_eth_leverage INTEGER DEFAULT 5,
			altcoin_leverage INTEGER DEFAULT 5,
			trading_symbols TEXT DEFAULT '',
			use_coin_pool BOOLEAN DEFAULT 0,
			use_oi_top BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 用户表
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			otp_secret TEXT,
			otp_verified BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 系统配置表
		`CREATE TABLE IF NOT EXISTS system_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 内测码表
		`CREATE TABLE IF NOT EXISTS beta_codes (
			code TEXT PRIMARY KEY,
			used BOOLEAN DEFAULT 0,
			used_by TEXT DEFAULT '',
			used_at DATETIME DEFAULT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 决策记录表
		`CREATE TABLE IF NOT EXISTS decision_records (
			id TEXT PRIMARY KEY,
			trader_id TEXT NOT NULL,
			cycle_number INTEGER NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			system_prompt TEXT,
			input_prompt TEXT,
			cot_trace TEXT,
			decision_json TEXT,
			account_state_json TEXT,
			positions_json TEXT,
			candidate_coins_json TEXT,
			execution_log_json TEXT,
			success BOOLEAN DEFAULT 0,
			error_message TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (trader_id) REFERENCES traders(id) ON DELETE CASCADE
		)`,

		// 交易记录表 - 存储每笔完整交易
		`CREATE TABLE IF NOT EXISTS trades (
			id TEXT PRIMARY KEY,
			trader_id TEXT NOT NULL,
			symbol TEXT NOT NULL,
			side TEXT NOT NULL, -- 'long' 或 'short'
			quantity REAL NOT NULL,
			leverage INTEGER NOT NULL,
			open_price REAL NOT NULL,
			close_price REAL,
			position_value REAL NOT NULL,
			margin_used REAL NOT NULL,
			pnl REAL DEFAULT 0,
			pnl_pct REAL DEFAULT 0,
			duration_seconds INTEGER DEFAULT 0,
			open_time DATETIME NOT NULL,
			close_time DATETIME,
			status TEXT NOT NULL DEFAULT 'open', -- 'open', 'closed', 'liquidated'
			close_reason TEXT DEFAULT '', -- 'manual', 'stop_loss', 'take_profit', 'liquidation'
			open_order_id TEXT,
			close_order_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (trader_id) REFERENCES traders(id) ON DELETE CASCADE
		)`,

		// 交易动作表 - 存储每个交易动作
		`CREATE TABLE IF NOT EXISTS trade_actions (
			id TEXT PRIMARY KEY,
			trader_id TEXT NOT NULL,
			decision_record_id TEXT,
			trade_id TEXT,
			action TEXT NOT NULL, -- 'open_long', 'open_short', 'close_long', 'close_short', 'stop_loss_long', 'stop_loss_short'
			symbol TEXT NOT NULL,
			quantity REAL NOT NULL,
			price REAL NOT NULL,
			leverage INTEGER DEFAULT 1,
			order_id TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			success BOOLEAN DEFAULT 0,
			error_message TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (trader_id) REFERENCES traders(id) ON DELETE CASCADE,
			FOREIGN KEY (decision_record_id) REFERENCES decision_records(id) ON DELETE SET NULL,
			FOREIGN KEY (trade_id) REFERENCES trades(id) ON DELETE SET NULL
		)`,

		// 触发器：自动更新 updated_at
		`CREATE TRIGGER IF NOT EXISTS update_users_updated_at
			AFTER UPDATE ON users
			BEGIN
				UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
			END`,

		`CREATE TRIGGER IF NOT EXISTS update_ai_models_updated_at
			AFTER UPDATE ON ai_models
			BEGIN
				UPDATE ai_models SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
			END`,

		`CREATE TRIGGER IF NOT EXISTS update_exchanges_updated_at
			AFTER UPDATE ON exchanges
			BEGIN
				UPDATE exchanges SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
			END`,

		`CREATE TRIGGER IF NOT EXISTS update_traders_updated_at
			AFTER UPDATE ON traders
			BEGIN
				UPDATE traders SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
			END`,

		`CREATE TRIGGER IF NOT EXISTS update_user_signal_sources_updated_at
			AFTER UPDATE ON user_signal_sources
			BEGIN
				UPDATE user_signal_sources SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
			END`,

		`CREATE TRIGGER IF NOT EXISTS update_system_config_updated_at
			AFTER UPDATE ON system_config
			BEGIN
				UPDATE system_config SET updated_at = CURRENT_TIMESTAMP WHERE key = NEW.key;
			END`,

		`CREATE TRIGGER IF NOT EXISTS update_trades_updated_at
			AFTER UPDATE ON trades
			BEGIN
				UPDATE trades SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
			END`,
	}

	for _, query := range queries {
		if _, err := d.db.Exec(query); err != nil {
			return fmt.Errorf("执行SQL失败 [%s]: %w", query, err)
		}
	}

	// 为现有数据库添加新字段（向后兼容）
	alterQueries := []string{
		`ALTER TABLE exchanges ADD COLUMN hyperliquid_wallet_addr TEXT DEFAULT ''`,
		`ALTER TABLE exchanges ADD COLUMN aster_user TEXT DEFAULT ''`,
		`ALTER TABLE exchanges ADD COLUMN aster_signer TEXT DEFAULT ''`,
		`ALTER TABLE exchanges ADD COLUMN aster_private_key TEXT DEFAULT ''`,
		`ALTER TABLE traders ADD COLUMN custom_prompt TEXT DEFAULT ''`,
		`ALTER TABLE traders ADD COLUMN override_base_prompt BOOLEAN DEFAULT 0`,
		`ALTER TABLE traders ADD COLUMN is_cross_margin BOOLEAN DEFAULT 1`,             // 默认为全仓模式
		`ALTER TABLE traders ADD COLUMN use_default_coins BOOLEAN DEFAULT 1`,           // 默认使用默认币种
		`ALTER TABLE traders ADD COLUMN custom_coins TEXT DEFAULT ''`,                  // 自定义币种列表（JSON格式）
		`ALTER TABLE traders ADD COLUMN btc_eth_leverage INTEGER DEFAULT 5`,            // BTC/ETH杠杆倍数
		`ALTER TABLE traders ADD COLUMN altcoin_leverage INTEGER DEFAULT 5`,            // 山寨币杠杆倍数
		`ALTER TABLE traders ADD COLUMN trading_symbols TEXT DEFAULT ''`,               // 交易币种，逗号分隔
		`ALTER TABLE traders ADD COLUMN use_coin_pool BOOLEAN DEFAULT 0`,               // 是否使用COIN POOL信号源
		`ALTER TABLE traders ADD COLUMN use_oi_top BOOLEAN DEFAULT 0`,                  // 是否使用OI TOP信号源
		`ALTER TABLE traders ADD COLUMN system_prompt_template TEXT DEFAULT 'default'`, // 系统提示词模板名称
		`ALTER TABLE ai_models ADD COLUMN custom_api_url TEXT DEFAULT ''`,              // 自定义API地址
		`ALTER TABLE ai_models ADD COLUMN custom_model_name TEXT DEFAULT ''`,           // 自定义模型名称
	}

	for _, query := range alterQueries {
		// 忽略已存在字段的错误
		d.db.Exec(query)
	}

	// 检查是否需要迁移exchanges表的主键结构
	err := d.migrateExchangesTable()
	if err != nil {
		log.Printf("⚠️ 迁移exchanges表失败: %v", err)
	}

	return nil
}

// verifyTableStructure 验证数据库表结构
func (d *Database) verifyTableStructure() error {
	log.Printf("🔍 验证数据库表结构...")
	
	// 验证traders表的列
	rows, err := d.db.Query("PRAGMA table_info(traders)")
	if err != nil {
		return fmt.Errorf("获取traders表信息失败: %w", err)
	}
	defer rows.Close()
	
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue interface{}
		var pk int
		
		err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk)
		if err != nil {
			continue
		}
		columns[name] = true
	}
	
	// 检查必需的列
	requiredColumns := []string{
		"id", "user_id", "name", "ai_model_id", "exchange_id", "initial_balance",
		"scan_interval_minutes", "is_running", "btc_eth_leverage", "altcoin_leverage",
		"trading_symbols", "use_coin_pool", "use_oi_top", "custom_prompt",
		"override_base_prompt", "system_prompt_template", "is_cross_margin",
	}
	
	missing := []string{}
	for _, col := range requiredColumns {
		if !columns[col] {
			missing = append(missing, col)
		}
	}
	
	if len(missing) > 0 {
		log.Printf("⚠️ traders表缺少列: %v", missing)
		log.Printf("📋 现有列: %v", getKeys(columns))
	} else {
		log.Printf("✅ traders表结构完整")
	}
	
	return nil
}

// getKeys 获取map的键列表
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// initDefaultData 初始化默认数据
func (d *Database) initDefaultData() error {
	// 初始化AI模型（使用default用户）
	aiModels := []struct {
		id, name, provider string
	}{
		{"deepseek", "DeepSeek", "deepseek"},
		{"qwen", "Qwen", "qwen"},
	}

	for _, model := range aiModels {
		_, err := d.db.Exec(`
			INSERT OR IGNORE INTO ai_models (id, user_id, name, provider, enabled) 
			VALUES (?, 'default', ?, ?, 0)
		`, model.id, model.name, model.provider)
		if err != nil {
			return fmt.Errorf("初始化AI模型失败: %w", err)
		}
	}

	// 初始化交易所（使用default用户）
	exchanges := []struct {
		id, name, typ string
	}{
		{"binance", "Binance Futures", "binance"},
		{"hyperliquid", "Hyperliquid", "hyperliquid"},
		{"aster", "Aster DEX", "aster"},
	}

	for _, exchange := range exchanges {
		_, err := d.db.Exec(`
			INSERT OR IGNORE INTO exchanges (id, user_id, name, type, enabled) 
			VALUES (?, 'default', ?, ?, 0)
		`, exchange.id, exchange.name, exchange.typ)
		if err != nil {
			return fmt.Errorf("初始化交易所失败: %w", err)
		}
	}

	// 初始化系统配置 - 创建所有字段，设置默认值，后续由config.json同步更新
	systemConfigs := map[string]string{
		"admin_mode":            "true",                                                                                // 默认开启管理员模式，便于首次使用
		"beta_mode":             "false",                                                                             // 默认关闭内测模式
		"api_server_port":       "8080",                                                                                // 默认API端口
		"use_default_coins":     "true",                                                                                // 默认使用内置币种列表
		"default_coins":         `["BTCUSDT","ETHUSDT","SOLUSDT","BNBUSDT","XRPUSDT","DOGEUSDT","ADAUSDT","HYPEUSDT"]`, // 默认币种列表（JSON格式）
		"max_daily_loss":        "10.0",                                                                                // 最大日损失百分比
		"max_drawdown":          "20.0",                                                                                // 最大回撤百分比
		"stop_trading_minutes":  "60",                                                                                  // 停止交易时间（分钟）
		"btc_eth_leverage":      "5",                                                                                   // BTC/ETH杠杆倍数
		"altcoin_leverage":      "5",                                                                                   // 山寨币杠杆倍数
		"jwt_secret":            "",                                                                                    // JWT密钥，默认为空，由config.json或系统生成
	}

	for key, value := range systemConfigs {
		_, err := d.db.Exec(`
			INSERT OR IGNORE INTO system_config (key, value) 
			VALUES (?, ?)
		`, key, value)
		if err != nil {
			return fmt.Errorf("初始化系统配置失败: %w", err)
		}
	}

	return nil
}

// migrateExchangesTable 迁移exchanges表支持多用户
func (d *Database) migrateExchangesTable() error {
	// 检查是否已经迁移过
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name='exchanges_new'
	`).Scan(&count)
	if err != nil {
		return err
	}

	// 如果已经迁移过，直接返回
	if count > 0 {
		return nil
	}

	log.Printf("🔄 开始迁移exchanges表...")

	// 创建新的exchanges表，使用复合主键
	_, err = d.db.Exec(`
		CREATE TABLE exchanges_new (
			id TEXT NOT NULL,
			user_id TEXT NOT NULL DEFAULT 'default',
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			enabled BOOLEAN DEFAULT 0,
			api_key TEXT DEFAULT '',
			secret_key TEXT DEFAULT '',
			testnet BOOLEAN DEFAULT 0,
			hyperliquid_wallet_addr TEXT DEFAULT '',
			aster_user TEXT DEFAULT '',
			aster_signer TEXT DEFAULT '',
			aster_private_key TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id, user_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("创建新exchanges表失败: %w", err)
	}

	// 复制数据到新表
	_, err = d.db.Exec(`
		INSERT INTO exchanges_new 
		SELECT * FROM exchanges
	`)
	if err != nil {
		return fmt.Errorf("复制数据失败: %w", err)
	}

	// 删除旧表
	_, err = d.db.Exec(`DROP TABLE exchanges`)
	if err != nil {
		return fmt.Errorf("删除旧表失败: %w", err)
	}

	// 重命名新表
	_, err = d.db.Exec(`ALTER TABLE exchanges_new RENAME TO exchanges`)
	if err != nil {
		return fmt.Errorf("重命名表失败: %w", err)
	}

	// 重新创建触发器
	_, err = d.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS update_exchanges_updated_at
			AFTER UPDATE ON exchanges
			BEGIN
				UPDATE exchanges SET updated_at = CURRENT_TIMESTAMP 
				WHERE id = NEW.id AND user_id = NEW.user_id;
			END
	`)
	if err != nil {
		return fmt.Errorf("创建触发器失败: %w", err)
	}

	log.Printf("✅ exchanges表迁移完成")
	return nil
}

// User 用户配置
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // 不返回到前端
	OTPSecret    string    `json:"-"` // 不返回到前端
	OTPVerified  bool      `json:"otp_verified"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AIModelConfig AI模型配置
type AIModelConfig struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Name            string    `json:"name"`
	Provider        string    `json:"provider"`
	Enabled         bool      `json:"enabled"`
	APIKey          string    `json:"apiKey"`
	CustomAPIURL    string    `json:"customApiUrl"`
	CustomModelName string    `json:"customModelName"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ExchangeConfig 交易所配置
type ExchangeConfig struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Enabled   bool   `json:"enabled"`
	APIKey    string `json:"apiKey"`
	SecretKey string `json:"secretKey"`
	Testnet   bool   `json:"testnet"`
	// Hyperliquid 特定字段
	HyperliquidWalletAddr string `json:"hyperliquidWalletAddr"`
	// Aster 特定字段
	AsterUser       string    `json:"asterUser"`
	AsterSigner     string    `json:"asterSigner"`
	AsterPrivateKey string    `json:"asterPrivateKey"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TraderRecord 交易员配置（数据库实体）
type TraderRecord struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	Name                 string    `json:"name"`
	AIModelID            string    `json:"ai_model_id"`
	ExchangeID           string    `json:"exchange_id"`
	InitialBalance       float64   `json:"initial_balance"`
	ScanIntervalMinutes  int       `json:"scan_interval_minutes"`
	IsRunning            bool      `json:"is_running"`
	BTCETHLeverage       int       `json:"btc_eth_leverage"`       // BTC/ETH杠杆倍数
	AltcoinLeverage      int       `json:"altcoin_leverage"`       // 山寨币杠杆倍数
	TradingSymbols       string    `json:"trading_symbols"`        // 交易币种，逗号分隔
	UseCoinPool          bool      `json:"use_coin_pool"`          // 是否使用COIN POOL信号源
	UseOITop             bool      `json:"use_oi_top"`             // 是否使用OI TOP信号源
	CustomPrompt         string    `json:"custom_prompt"`          // 自定义交易策略prompt
	OverrideBasePrompt   bool      `json:"override_base_prompt"`   // 是否覆盖基础prompt
	SystemPromptTemplate string    `json:"system_prompt_template"` // 系统提示词模板名称
	IsCrossMargin        bool      `json:"is_cross_margin"`        // 是否为全仓模式（true=全仓，false=逐仓）
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// UserSignalSource 用户信号源配置
type UserSignalSource struct {
	ID          int       `json:"id"`
	UserID      string    `json:"user_id"`
	CoinPoolURL string    `json:"coin_pool_url"`
	OITopURL    string    `json:"oi_top_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GenerateOTPSecret 生成OTP密钥
func GenerateOTPSecret() (string, error) {
	secret := make([]byte, 20)
	_, err := rand.Read(secret)
	if err != nil {
		return "", err
	}
	return base32.StdEncoding.EncodeToString(secret), nil
}

// CreateUser 创建用户
func (d *Database) CreateUser(user *User) error {
	_, err := d.db.Exec(`
		INSERT INTO users (id, email, password_hash, otp_secret, otp_verified)
		VALUES (?, ?, ?, ?, ?)
	`, user.ID, user.Email, user.PasswordHash, user.OTPSecret, user.OTPVerified)
	return err
}

// EnsureAdminUser 确保admin用户存在（用于管理员模式）
func (d *Database) EnsureAdminUser() error {
	// 检查admin用户是否已存在
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = 'admin'`).Scan(&count)
	if err != nil {
		return err
	}

	// 如果已存在，直接返回
	if count > 0 {
		return nil
	}

	// 创建admin用户（密码为空，因为管理员模式下不需要密码）
	adminUser := &User{
		ID:           "admin",
		Email:        "admin@localhost",
		PasswordHash: "", // 管理员模式下不使用密码
		OTPSecret:    "",
		OTPVerified:  true,
	}

	return d.CreateUser(adminUser)
}

// GetUserByEmail 通过邮箱获取用户
func (d *Database) GetUserByEmail(email string) (*User, error) {
	var user User
	err := d.db.QueryRow(`
		SELECT id, email, password_hash, otp_secret, otp_verified, created_at, updated_at
		FROM users WHERE email = ?
	`, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.OTPSecret,
		&user.OTPVerified, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByID 通过ID获取用户
func (d *Database) GetUserByID(userID string) (*User, error) {
	var user User
	err := d.db.QueryRow(`
		SELECT id, email, password_hash, otp_secret, otp_verified, created_at, updated_at
		FROM users WHERE id = ?
	`, userID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.OTPSecret,
		&user.OTPVerified, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetAllUsers 获取所有用户ID列表
func (d *Database) GetAllUsers() ([]string, error) {
	rows, err := d.db.Query(`SELECT id FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, nil
}

// UpdateUserOTPVerified 更新用户OTP验证状态
func (d *Database) UpdateUserOTPVerified(userID string, verified bool) error {
	_, err := d.db.Exec(`UPDATE users SET otp_verified = ? WHERE id = ?`, verified, userID)
	return err
}

// GetAIModels 获取用户的AI模型配置
func (d *Database) GetAIModels(userID string) ([]*AIModelConfig, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, name, provider, enabled, api_key,
		       COALESCE(custom_api_url, '') as custom_api_url,
		       COALESCE(custom_model_name, '') as custom_model_name,
		       created_at, updated_at
		FROM ai_models WHERE user_id = ? ORDER BY id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 初始化为空切片而不是nil，确保JSON序列化为[]而不是null
	models := make([]*AIModelConfig, 0)
	for rows.Next() {
		var model AIModelConfig
		err := rows.Scan(
			&model.ID, &model.UserID, &model.Name, &model.Provider,
			&model.Enabled, &model.APIKey, &model.CustomAPIURL, &model.CustomModelName,
			&model.CreatedAt, &model.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		models = append(models, &model)
	}

	return models, nil
}

// UpdateAIModel 更新AI模型配置，如果不存在则创建用户特定配置
func (d *Database) UpdateAIModel(userID, id string, enabled bool, apiKey, customAPIURL, customModelName string) error {
	// 先尝试精确匹配 ID（新版逻辑，支持多个相同 provider 的模型）
	var existingID string
	err := d.db.QueryRow(`
		SELECT id FROM ai_models WHERE user_id = ? AND id = ? LIMIT 1
	`, userID, id).Scan(&existingID)

	if err == nil {
		// 找到了现有配置（精确匹配 ID），更新它
		_, err = d.db.Exec(`
			UPDATE ai_models SET enabled = ?, api_key = ?, custom_api_url = ?, custom_model_name = ?, updated_at = datetime('now')
			WHERE id = ? AND user_id = ?
		`, enabled, apiKey, customAPIURL, customModelName, existingID, userID)
		return err
	}

	// ID 不存在，尝试兼容旧逻辑：将 id 作为 provider 查找
	provider := id
	err = d.db.QueryRow(`
		SELECT id FROM ai_models WHERE user_id = ? AND provider = ? LIMIT 1
	`, userID, provider).Scan(&existingID)

	if err == nil {
		// 找到了现有配置（通过 provider 匹配，兼容旧版），更新它
		log.Printf("⚠️  使用旧版 provider 匹配更新模型: %s -> %s", provider, existingID)
		_, err = d.db.Exec(`
			UPDATE ai_models SET enabled = ?, api_key = ?, custom_api_url = ?, custom_model_name = ?, updated_at = datetime('now')
			WHERE id = ? AND user_id = ?
		`, enabled, apiKey, customAPIURL, customModelName, existingID, userID)
		return err
	}

	// 没有找到任何现有配置，创建新的
	// 推断 provider（从 id 中提取，或者直接使用 id）
	if provider == id && (provider == "deepseek" || provider == "qwen") {
		// id 本身就是 provider
		provider = id
	} else {
		// 从 id 中提取 provider（假设格式是 userID_provider 或 timestamp_userID_provider）
		parts := strings.Split(id, "_")
		if len(parts) >= 2 {
			provider = parts[len(parts)-1] // 取最后一部分作为 provider
		} else {
			provider = id
		}
	}

	// 获取模型的基本信息
	var name string
	err = d.db.QueryRow(`
		SELECT name FROM ai_models WHERE provider = ? LIMIT 1
	`, provider).Scan(&name)
	if err != nil {
		// 如果找不到基本信息，使用默认值
		if provider == "deepseek" {
			name = "DeepSeek AI"
		} else if provider == "qwen" {
			name = "Qwen AI"
		} else {
			name = provider + " AI"
		}
	}

	// 如果传入的 ID 已经是完整格式（如 "admin_deepseek_custom1"），直接使用
	// 否则生成新的 ID
	newModelID := id
	if id == provider {
		// id 就是 provider，生成新的用户特定 ID
		newModelID = fmt.Sprintf("%s_%s", userID, provider)
	}

	log.Printf("✓ 创建新的 AI 模型配置: ID=%s, Provider=%s, Name=%s", newModelID, provider, name)
	_, err = d.db.Exec(`
		INSERT INTO ai_models (id, user_id, name, provider, enabled, api_key, custom_api_url, custom_model_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, newModelID, userID, name, provider, enabled, apiKey, customAPIURL, customModelName)

	return err
}

// GetExchanges 获取用户的交易所配置
func (d *Database) GetExchanges(userID string) ([]*ExchangeConfig, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, name, type, enabled, api_key, secret_key, testnet, 
		       COALESCE(hyperliquid_wallet_addr, '') as hyperliquid_wallet_addr,
		       COALESCE(aster_user, '') as aster_user,
		       COALESCE(aster_signer, '') as aster_signer,
		       COALESCE(aster_private_key, '') as aster_private_key,
		       created_at, updated_at 
		FROM exchanges WHERE user_id = ? ORDER BY id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 初始化为空切片而不是nil，确保JSON序列化为[]而不是null
	exchanges := make([]*ExchangeConfig, 0)
	for rows.Next() {
		var exchange ExchangeConfig
		err := rows.Scan(
			&exchange.ID, &exchange.UserID, &exchange.Name, &exchange.Type,
			&exchange.Enabled, &exchange.APIKey, &exchange.SecretKey, &exchange.Testnet,
			&exchange.HyperliquidWalletAddr, &exchange.AsterUser,
			&exchange.AsterSigner, &exchange.AsterPrivateKey,
			&exchange.CreatedAt, &exchange.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		exchanges = append(exchanges, &exchange)
	}

	return exchanges, nil
}

// UpdateExchange 更新交易所配置，如果不存在则创建用户特定配置
func (d *Database) UpdateExchange(userID, id string, enabled bool, apiKey, secretKey string, testnet bool, hyperliquidWalletAddr, asterUser, asterSigner, asterPrivateKey string) error {
	log.Printf("🔧 UpdateExchange: userID=%s, id=%s, enabled=%v", userID, id, enabled)

	// 首先尝试更新现有的用户配置
	result, err := d.db.Exec(`
		UPDATE exchanges SET enabled = ?, api_key = ?, secret_key = ?, testnet = ?, 
		       hyperliquid_wallet_addr = ?, aster_user = ?, aster_signer = ?, aster_private_key = ?, updated_at = datetime('now')
		WHERE id = ? AND user_id = ?
	`, enabled, apiKey, secretKey, testnet, hyperliquidWalletAddr, asterUser, asterSigner, asterPrivateKey, id, userID)
	if err != nil {
		log.Printf("❌ UpdateExchange: 更新失败: %v", err)
		return err
	}

	// 检查是否有行被更新
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("❌ UpdateExchange: 获取影响行数失败: %v", err)
		return err
	}

	log.Printf("📊 UpdateExchange: 影响行数 = %d", rowsAffected)

	// 如果没有行被更新，说明用户没有这个交易所的配置，需要创建
	if rowsAffected == 0 {
		log.Printf("💡 UpdateExchange: 没有现有记录，创建新记录")

		// 根据交易所ID确定基本信息
		var name, typ string
		if id == "binance" {
			name = "Binance Futures"
			typ = "cex"
		} else if id == "hyperliquid" {
			name = "Hyperliquid"
			typ = "dex"
		} else if id == "aster" {
			name = "Aster DEX"
			typ = "dex"
		} else {
			name = id + " Exchange"
			typ = "cex"
		}

		log.Printf("🆕 UpdateExchange: 创建新记录 ID=%s, name=%s, type=%s", id, name, typ)

		// 创建用户特定的配置，使用原始的交易所ID
		_, err = d.db.Exec(`
			INSERT INTO exchanges (id, user_id, name, type, enabled, api_key, secret_key, testnet, 
			                       hyperliquid_wallet_addr, aster_user, aster_signer, aster_private_key, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		`, id, userID, name, typ, enabled, apiKey, secretKey, testnet, hyperliquidWalletAddr, asterUser, asterSigner, asterPrivateKey)

		if err != nil {
			log.Printf("❌ UpdateExchange: 创建记录失败: %v", err)
		} else {
			log.Printf("✅ UpdateExchange: 创建记录成功")
		}
		return err
	}

	log.Printf("✅ UpdateExchange: 更新现有记录成功")
	return nil
}

// CreateAIModel 创建AI模型配置
func (d *Database) CreateAIModel(userID, id, name, provider string, enabled bool, apiKey, customAPIURL string) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO ai_models (id, user_id, name, provider, enabled, api_key, custom_api_url) 
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, userID, name, provider, enabled, apiKey, customAPIURL)
	return err
}

// CreateExchange 创建交易所配置
func (d *Database) CreateExchange(userID, id, name, typ string, enabled bool, apiKey, secretKey string, testnet bool, hyperliquidWalletAddr, asterUser, asterSigner, asterPrivateKey string) error {
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO exchanges (id, user_id, name, type, enabled, api_key, secret_key, testnet, hyperliquid_wallet_addr, aster_user, aster_signer, aster_private_key) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, userID, name, typ, enabled, apiKey, secretKey, testnet, hyperliquidWalletAddr, asterUser, asterSigner, asterPrivateKey)
	return err
}

// CreateTrader 创建交易员
func (d *Database) CreateTrader(trader *TraderRecord) error {
	log.Printf("🆕 正在创建交易员: ID=%s, Name=%s, UserID=%s", trader.ID, trader.Name, trader.UserID)
	log.Printf("   AI模型: %s, 交易所: %s", trader.AIModelID, trader.ExchangeID)
	log.Printf("   杠杆: BTC/ETH=%d, 山寨币=%d", trader.BTCETHLeverage, trader.AltcoinLeverage)
	log.Printf("   交易币种: '%s'", trader.TradingSymbols)
	
	// 检查外键约束 - 验证用户是否存在
	var userExists bool
	err := d.db.QueryRow("SELECT 1 FROM users WHERE id = ?", trader.UserID).Scan(&userExists)
	if err != nil {
		log.Printf("⚠️ 用户 %s 不存在，但继续创建交易员（可能使用管理员模式）", trader.UserID)
	}
	
	// 检查AI模型是否存在（更宽松的检查）
	var aiModelExists bool
	err = d.db.QueryRow("SELECT 1 FROM ai_models WHERE id = ? AND user_id = ?", trader.AIModelID, trader.UserID).Scan(&aiModelExists)
	if err != nil {
		log.Printf("⚠️ AI模型 %s 对用户 %s 不存在，尝试查找通用模型", trader.AIModelID, trader.UserID)
		// 检查是否是通用模型（default用户的模型）
		err = d.db.QueryRow("SELECT 1 FROM ai_models WHERE provider = ?", trader.AIModelID).Scan(&aiModelExists)
		if err != nil {
			log.Printf("⚠️ 未找到匹配的AI模型: %s", trader.AIModelID)
		}
	}
	
	// 检查交易所是否存在（更宽松的检查）
	var exchangeExists bool
	err = d.db.QueryRow("SELECT 1 FROM exchanges WHERE id = ? AND user_id = ?", trader.ExchangeID, trader.UserID).Scan(&exchangeExists)
	if err != nil {
		log.Printf("⚠️ 交易所 %s 对用户 %s 不存在，尝试查找通用交易所", trader.ExchangeID, trader.UserID)
		// 检查是否是通用交易所（default用户的交易所）
		err = d.db.QueryRow("SELECT 1 FROM exchanges WHERE id = ?", trader.ExchangeID).Scan(&exchangeExists)
		if err != nil {
			log.Printf("⚠️ 未找到匹配的交易所: %s", trader.ExchangeID)
		}
	}
	
	result, err := d.db.Exec(`
		INSERT INTO traders (
			id, user_id, name, ai_model_id, exchange_id, initial_balance, 
			scan_interval_minutes, is_running, btc_eth_leverage, altcoin_leverage, 
			trading_symbols, use_coin_pool, use_oi_top, custom_prompt, 
			override_base_prompt, system_prompt_template, is_cross_margin
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, trader.ID, trader.UserID, trader.Name, trader.AIModelID, trader.ExchangeID, 
		trader.InitialBalance, trader.ScanIntervalMinutes, trader.IsRunning, 
		trader.BTCETHLeverage, trader.AltcoinLeverage, trader.TradingSymbols, 
		trader.UseCoinPool, trader.UseOITop, trader.CustomPrompt, 
		trader.OverrideBasePrompt, trader.SystemPromptTemplate, trader.IsCrossMargin)
	
	if err != nil {
		log.Printf("❌ 创建交易员失败: %v", err)
		log.Printf("   可能原因: 1)表结构不完整 2)外键约束失败 3)字段值无效")
		return err
	}
	
	rowsAffected, _ := result.RowsAffected()
	log.Printf("✅ 交易员创建成功，影响行数: %d", rowsAffected)
	
	// 验证数据是否真的插入了
	var verifyID string
	err = d.db.QueryRow("SELECT id FROM traders WHERE id = ?", trader.ID).Scan(&verifyID)
	if err != nil {
		log.Printf("⚠️ 验证失败: 创建的交易员在数据库中找不到: %v", err)
	} else {
		log.Printf("✅ 验证成功: 交易员 %s 已存在于数据库中", verifyID)
	}
	
	return nil
}

// GetTraders 获取用户的交易员
func (d *Database) GetTraders(userID string) ([]*TraderRecord, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, name, ai_model_id, exchange_id, initial_balance, scan_interval_minutes, is_running,
		       COALESCE(btc_eth_leverage, 5) as btc_eth_leverage, COALESCE(altcoin_leverage, 5) as altcoin_leverage,
		       COALESCE(trading_symbols, '') as trading_symbols,
		       COALESCE(use_coin_pool, 0) as use_coin_pool, COALESCE(use_oi_top, 0) as use_oi_top,
		       COALESCE(custom_prompt, '') as custom_prompt, COALESCE(override_base_prompt, 0) as override_base_prompt,
		       COALESCE(system_prompt_template, 'default') as system_prompt_template,
		       COALESCE(is_cross_margin, 1) as is_cross_margin, created_at, updated_at
		FROM traders WHERE user_id = ? ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traders []*TraderRecord
	for rows.Next() {
		var trader TraderRecord
		err := rows.Scan(
			&trader.ID, &trader.UserID, &trader.Name, &trader.AIModelID, &trader.ExchangeID,
			&trader.InitialBalance, &trader.ScanIntervalMinutes, &trader.IsRunning,
			&trader.BTCETHLeverage, &trader.AltcoinLeverage, &trader.TradingSymbols,
			&trader.UseCoinPool, &trader.UseOITop,
			&trader.CustomPrompt, &trader.OverrideBasePrompt, &trader.SystemPromptTemplate,
			&trader.IsCrossMargin,
			&trader.CreatedAt, &trader.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		traders = append(traders, &trader)
	}

	return traders, nil
}

// UpdateTraderStatus 更新交易员状态
func (d *Database) UpdateTraderStatus(userID, id string, isRunning bool) error {
	_, err := d.db.Exec(`UPDATE traders SET is_running = ? WHERE id = ? AND user_id = ?`, isRunning, id, userID)
	return err
}

// UpdateTrader 更新交易员配置
func (d *Database) UpdateTrader(trader *TraderRecord) error {
	_, err := d.db.Exec(`
		UPDATE traders SET
			name = ?, ai_model_id = ?, exchange_id = ?, initial_balance = ?,
			scan_interval_minutes = ?, btc_eth_leverage = ?, altcoin_leverage = ?,
			trading_symbols = ?, custom_prompt = ?, override_base_prompt = ?,
			system_prompt_template = ?, is_cross_margin = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`, trader.Name, trader.AIModelID, trader.ExchangeID, trader.InitialBalance,
		trader.ScanIntervalMinutes, trader.BTCETHLeverage, trader.AltcoinLeverage,
		trader.TradingSymbols, trader.CustomPrompt, trader.OverrideBasePrompt,
		trader.SystemPromptTemplate, trader.IsCrossMargin, trader.ID, trader.UserID)
	return err
}

// UpdateTraderCustomPrompt 更新交易员自定义Prompt
func (d *Database) UpdateTraderCustomPrompt(userID, id string, customPrompt string, overrideBase bool) error {
	_, err := d.db.Exec(`UPDATE traders SET custom_prompt = ?, override_base_prompt = ? WHERE id = ? AND user_id = ?`, customPrompt, overrideBase, id, userID)
	return err
}

// DeleteTrader 删除交易员
func (d *Database) DeleteTrader(userID, id string) error {
	_, err := d.db.Exec(`DELETE FROM traders WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// GetTraderConfig 获取交易员完整配置（包含AI模型和交易所信息）
func (d *Database) GetTraderConfig(userID, traderID string) (*TraderRecord, *AIModelConfig, *ExchangeConfig, error) {
	var trader TraderRecord
	var aiModel AIModelConfig
	var exchange ExchangeConfig

	err := d.db.QueryRow(`
		SELECT 
			t.id, t.user_id, t.name, t.ai_model_id, t.exchange_id, t.initial_balance, t.scan_interval_minutes, t.is_running,
			COALESCE(t.btc_eth_leverage, 5) as btc_eth_leverage, 
			COALESCE(t.altcoin_leverage, 5) as altcoin_leverage,
			COALESCE(t.trading_symbols, '') as trading_symbols,
			COALESCE(t.use_coin_pool, 0) as use_coin_pool, 
			COALESCE(t.use_oi_top, 0) as use_oi_top,
			COALESCE(t.custom_prompt, '') as custom_prompt, 
			COALESCE(t.override_base_prompt, 0) as override_base_prompt,
			COALESCE(t.system_prompt_template, 'default') as system_prompt_template,
			COALESCE(t.is_cross_margin, 1) as is_cross_margin,
			t.created_at, t.updated_at,
			a.id, a.user_id, a.name, a.provider, a.enabled, a.api_key,
			COALESCE(a.custom_api_url, '') as custom_api_url,
			COALESCE(a.custom_model_name, '') as custom_model_name,
			a.created_at, a.updated_at,
			e.id, e.user_id, e.name, e.type, e.enabled, e.api_key, e.secret_key, e.testnet,
			COALESCE(e.hyperliquid_wallet_addr, '') as hyperliquid_wallet_addr,
			COALESCE(e.aster_user, '') as aster_user,
			COALESCE(e.aster_signer, '') as aster_signer,
			COALESCE(e.aster_private_key, '') as aster_private_key,
			e.created_at, e.updated_at
		FROM traders t
		JOIN ai_models a ON t.ai_model_id = a.id AND t.user_id = a.user_id
		JOIN exchanges e ON t.exchange_id = e.id AND t.user_id = e.user_id
		WHERE t.id = ? AND t.user_id = ?
	`, traderID, userID).Scan(
		&trader.ID, &trader.UserID, &trader.Name, &trader.AIModelID, &trader.ExchangeID,
		&trader.InitialBalance, &trader.ScanIntervalMinutes, &trader.IsRunning,
		&trader.BTCETHLeverage, &trader.AltcoinLeverage, &trader.TradingSymbols,
		&trader.UseCoinPool, &trader.UseOITop,
		&trader.CustomPrompt, &trader.OverrideBasePrompt, &trader.SystemPromptTemplate,
		&trader.IsCrossMargin,
		&trader.CreatedAt, &trader.UpdatedAt,
		&aiModel.ID, &aiModel.UserID, &aiModel.Name, &aiModel.Provider, &aiModel.Enabled, &aiModel.APIKey,
		&aiModel.CustomAPIURL, &aiModel.CustomModelName,
		&aiModel.CreatedAt, &aiModel.UpdatedAt,
		&exchange.ID, &exchange.UserID, &exchange.Name, &exchange.Type, &exchange.Enabled,
		&exchange.APIKey, &exchange.SecretKey, &exchange.Testnet,
		&exchange.HyperliquidWalletAddr, &exchange.AsterUser, &exchange.AsterSigner, &exchange.AsterPrivateKey,
		&exchange.CreatedAt, &exchange.UpdatedAt,
	)

	if err != nil {
		return nil, nil, nil, err
	}

	return &trader, &aiModel, &exchange, nil
}

// GetSystemConfig 获取系统配置
func (d *Database) GetSystemConfig(key string) (string, error) {
	var value string
	err := d.db.QueryRow(`SELECT value FROM system_config WHERE key = ?`, key).Scan(&value)
	return value, err
}

// SetSystemConfig 设置系统配置
func (d *Database) SetSystemConfig(key, value string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO system_config (key, value) VALUES (?, ?)
	`, key, value)
	return err
}

// CreateUserSignalSource 创建用户信号源配置
func (d *Database) CreateUserSignalSource(userID, coinPoolURL, oiTopURL string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO user_signal_sources (user_id, coin_pool_url, oi_top_url, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, userID, coinPoolURL, oiTopURL)
	return err
}

// GetUserSignalSource 获取用户信号源配置
func (d *Database) GetUserSignalSource(userID string) (*UserSignalSource, error) {
	var source UserSignalSource
	err := d.db.QueryRow(`
		SELECT id, user_id, coin_pool_url, oi_top_url, created_at, updated_at
		FROM user_signal_sources WHERE user_id = ?
	`, userID).Scan(
		&source.ID, &source.UserID, &source.CoinPoolURL, &source.OITopURL,
		&source.CreatedAt, &source.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &source, nil
}

// UpdateUserSignalSource 更新用户信号源配置
func (d *Database) UpdateUserSignalSource(userID, coinPoolURL, oiTopURL string) error {
	_, err := d.db.Exec(`
		UPDATE user_signal_sources SET coin_pool_url = ?, oi_top_url = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ?
	`, coinPoolURL, oiTopURL, userID)
	return err
}

// GetCustomCoins 获取所有交易员自定义币种 / Get all trader-customized currencies
func (d *Database) GetCustomCoins() []string {
	var symbol string
	var symbols []string
	_ = d.db.QueryRow(`
		SELECT GROUP_CONCAT(custom_coins , ',') as symbol
		FROM main.traders where custom_coins != ''
	`).Scan(&symbol)
	// 检测用户是否未配置币种 - 兼容性
	if symbol == "" {
		symbolJSON, _ := d.GetSystemConfig("default_coins")
		if err := json.Unmarshal([]byte(symbolJSON), &symbols); err != nil {
			log.Printf("⚠️  解析default_coins配置失败: %v，使用硬编码默认值", err)
			symbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"}
		}
	}
	// filter Symbol
	for _, s := range strings.Split(symbol, ",") {
		if s == "" {
			continue
		}
		coin := market.Normalize(s)
		if !slices.Contains(symbols, coin) {
			symbols = append(symbols, coin)
		}
	}
	return symbols
}

// Close 关闭数据库连接
func (d *Database) Close() error {
	return d.db.Close()
}

// LoadBetaCodesFromFile 从文件加载内测码到数据库
func (d *Database) LoadBetaCodesFromFile(filePath string) error {
	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取内测码文件失败: %w", err)
	}

	// 按行分割内测码
	lines := strings.Split(string(content), "\n")
	var codes []string
	for _, line := range lines {
		code := strings.TrimSpace(line)
		if code != "" && !strings.HasPrefix(code, "#") {
			codes = append(codes, code)
		}
	}

	// 批量插入内测码
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO beta_codes (code) VALUES (?)`)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	insertedCount := 0
	for _, code := range codes {
		result, err := stmt.Exec(code)
		if err != nil {
			log.Printf("插入内测码 %s 失败: %v", code, err)
			continue
		}
		
		if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
			insertedCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	log.Printf("✅ 成功加载 %d 个内测码到数据库 (总计 %d 个)", insertedCount, len(codes))
	return nil
}

// ValidateBetaCode 验证内测码是否有效且未使用
func (d *Database) ValidateBetaCode(code string) (bool, error) {
	var used bool
	err := d.db.QueryRow(`SELECT used FROM beta_codes WHERE code = ?`, code).Scan(&used)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // 内测码不存在
		}
		return false, err
	}
	return !used, nil // 内测码存在且未使用
}

// UseBetaCode 使用内测码（标记为已使用）
func (d *Database) UseBetaCode(code, userEmail string) error {
	result, err := d.db.Exec(`
		UPDATE beta_codes SET used = 1, used_by = ?, used_at = CURRENT_TIMESTAMP 
		WHERE code = ? AND used = 0
	`, userEmail, code)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("内测码无效或已被使用")
	}

	return nil
}

// GetBetaCodeStats 获取内测码统计信息
func (d *Database) GetBetaCodeStats() (total, used int, err error) {
	err = d.db.QueryRow(`SELECT COUNT(*) FROM beta_codes`).Scan(&total)
	if err != nil {
		return 0, 0, err
	}

	err = d.db.QueryRow(`SELECT COUNT(*) FROM beta_codes WHERE used = 1`).Scan(&used)
	if err != nil {
		return 0, 0, err
	}

	return total, used, nil
}

// ===== 交易记录相关方法 =====

// TradeRecord 交易记录结构
type TradeRecord struct {
	ID            string    `json:"id"`
	TraderID      string    `json:"trader_id"`
	Symbol        string    `json:"symbol"`
	Side          string    `json:"side"` // 'long' 或 'short'
	Quantity      float64   `json:"quantity"`
	Leverage      int       `json:"leverage"`
	OpenPrice     float64   `json:"open_price"`
	ClosePrice    *float64  `json:"close_price"`
	PositionValue float64   `json:"position_value"`
	MarginUsed    float64   `json:"margin_used"`
	PnL           float64   `json:"pnl"`
	PnLPct        float64   `json:"pnl_pct"`
	DurationSecs  int       `json:"duration_seconds"`
	OpenTime      time.Time `json:"open_time"`
	CloseTime     *time.Time `json:"close_time"`
	Status        string    `json:"status"` // 'open', 'closed', 'liquidated'
	CloseReason   string    `json:"close_reason"` // 'manual', 'stop_loss', 'take_profit', 'liquidation'
	OpenOrderID   string    `json:"open_order_id"`
	CloseOrderID  string    `json:"close_order_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TradeAction 交易动作结构
type TradeActionRecord struct {
	ID               string    `json:"id"`
	TraderID         string    `json:"trader_id"`
	DecisionRecordID *string   `json:"decision_record_id"`
	TradeID          *string   `json:"trade_id"`
	Action           string    `json:"action"` // 'open_long', 'open_short', 'close_long', 'close_short', 'stop_loss_long', 'stop_loss_short'
	Symbol           string    `json:"symbol"`
	Quantity         float64   `json:"quantity"`
	Price            float64   `json:"price"`
	Leverage         int       `json:"leverage"`
	OrderID          string    `json:"order_id"`
	Timestamp        time.Time `json:"timestamp"`
	Success          bool      `json:"success"`
	ErrorMessage     string    `json:"error_message"`
	CreatedAt        time.Time `json:"created_at"`
}

// DecisionRecordDB 决策记录数据库结构
type DecisionRecordDB struct {
	ID                 string    `json:"id"`
	TraderID           string    `json:"trader_id"`
	CycleNumber        int       `json:"cycle_number"`
	Timestamp          time.Time `json:"timestamp"`
	SystemPrompt       string    `json:"system_prompt"`
	InputPrompt        string    `json:"input_prompt"`
	CoTTrace           string    `json:"cot_trace"`
	DecisionJSON       string    `json:"decision_json"`
	AccountStateJSON   string    `json:"account_state_json"`
	PositionsJSON      string    `json:"positions_json"`
	CandidateCoinsJSON string    `json:"candidate_coins_json"`
	ExecutionLogJSON   string    `json:"execution_log_json"`
	Success            bool      `json:"success"`
	ErrorMessage       string    `json:"error_message"`
	CreatedAt          time.Time `json:"created_at"`
}

// CreateTrade 创建新交易记录
func (d *Database) CreateTrade(trade *TradeRecord) error {
	if trade.ID == "" {
		trade.ID = fmt.Sprintf("trade_%d", time.Now().UnixNano())
	}
	
	_, err := d.db.Exec(`
		INSERT INTO trades (
			id, trader_id, symbol, side, quantity, leverage, open_price, 
			position_value, margin_used, open_time, status, open_order_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, trade.ID, trade.TraderID, trade.Symbol, trade.Side, trade.Quantity, 
	trade.Leverage, trade.OpenPrice, trade.PositionValue, trade.MarginUsed, 
	trade.OpenTime, trade.Status, trade.OpenOrderID)
	
	return err
}

// UpdateTrade 更新交易记录（主要用于平仓）
func (d *Database) UpdateTrade(tradeID string, closePrice float64, closeTime time.Time, 
	status, closeReason, closeOrderID string, pnl, pnlPct float64, durationSecs int) error {
	
	_, err := d.db.Exec(`
		UPDATE trades SET 
			close_price = ?, close_time = ?, status = ?, close_reason = ?, 
			close_order_id = ?, pnl = ?, pnl_pct = ?, duration_seconds = ?
		WHERE id = ?
	`, closePrice, closeTime, status, closeReason, closeOrderID, pnl, pnlPct, durationSecs, tradeID)
	
	return err
}

// GetOpenTrade 获取指定trader的开仓交易
func (d *Database) GetOpenTrade(traderID, symbol, side string) (*TradeRecord, error) {
	var trade TradeRecord
	var closePriceSql sql.NullFloat64
	var closeTime sql.NullTime
	var closeOrderID sql.NullString
	
	err := d.db.QueryRow(`
		SELECT id, trader_id, symbol, side, quantity, leverage, open_price, close_price,
			position_value, margin_used, pnl, pnl_pct, duration_seconds,
			open_time, close_time, status, close_reason, open_order_id, close_order_id,
			created_at, updated_at
		FROM trades 
		WHERE trader_id = ? AND symbol = ? AND side = ? AND status = 'open'
		ORDER BY open_time DESC LIMIT 1
	`, traderID, symbol, side).Scan(
		&trade.ID, &trade.TraderID, &trade.Symbol, &trade.Side, &trade.Quantity,
		&trade.Leverage, &trade.OpenPrice, &closePriceSql, &trade.PositionValue,
		&trade.MarginUsed, &trade.PnL, &trade.PnLPct, &trade.DurationSecs,
		&trade.OpenTime, &closeTime, &trade.Status, &trade.CloseReason,
		&trade.OpenOrderID, &closeOrderID, &trade.CreatedAt, &trade.UpdatedAt)
	
	if err != nil {
		return nil, err
	}
	
	if closePriceSql.Valid {
		trade.ClosePrice = &closePriceSql.Float64
	}
	if closeTime.Valid {
		trade.CloseTime = &closeTime.Time
	}
	if closeOrderID.Valid {
		trade.CloseOrderID = closeOrderID.String
	}
	
	return &trade, nil
}

// GetTraderTrades 获取指定trader的交易记录
func (d *Database) GetTraderTrades(traderID string, limit int) ([]*TradeRecord, error) {
	query := `
		SELECT id, trader_id, symbol, side, quantity, leverage, open_price, close_price,
			position_value, margin_used, pnl, pnl_pct, duration_seconds,
			open_time, close_time, status, close_reason, open_order_id, close_order_id,
			created_at, updated_at
		FROM trades 
		WHERE trader_id = ? 
		ORDER BY open_time DESC
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	
	rows, err := d.db.Query(query, traderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var trades []*TradeRecord
	for rows.Next() {
		var trade TradeRecord
		var closePrice sql.NullFloat64
		var closeTime sql.NullTime
		var closeOrderID sql.NullString
		
		err := rows.Scan(
			&trade.ID, &trade.TraderID, &trade.Symbol, &trade.Side, &trade.Quantity,
			&trade.Leverage, &trade.OpenPrice, &closePrice, &trade.PositionValue,
			&trade.MarginUsed, &trade.PnL, &trade.PnLPct, &trade.DurationSecs,
			&trade.OpenTime, &closeTime, &trade.Status, &trade.CloseReason,
			&trade.OpenOrderID, &closeOrderID, &trade.CreatedAt, &trade.UpdatedAt)
		
		if err != nil {
			log.Printf("⚠️ 扫描交易记录失败: %v", err)
			continue
		}
		
		if closePrice.Valid {
			trade.ClosePrice = &closePrice.Float64
		}
		if closeTime.Valid {
			trade.CloseTime = &closeTime.Time
		}
		if closeOrderID.Valid {
			trade.CloseOrderID = closeOrderID.String
		}
		
		trades = append(trades, &trade)
	}
	
	return trades, nil
}

// DeleteTrade 删除交易记录
func (d *Database) DeleteTrade(tradeID string) error {
	_, err := d.db.Exec(`DELETE FROM trades WHERE id = ?`, tradeID)
	return err
}

// DeleteTraderTrades 删除指定trader的所有交易记录
func (d *Database) DeleteTraderTrades(traderID string) error {
	_, err := d.db.Exec(`DELETE FROM trades WHERE trader_id = ?`, traderID)
	return err
}

// CreateTradeAction 创建交易动作记录
func (d *Database) CreateTradeAction(action *TradeActionRecord) error {
	if action.ID == "" {
		action.ID = fmt.Sprintf("action_%d", time.Now().UnixNano())
	}
	
	_, err := d.db.Exec(`
		INSERT INTO trade_actions (
			id, trader_id, decision_record_id, trade_id, action, symbol, 
			quantity, price, leverage, order_id, timestamp, success, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, action.ID, action.TraderID, action.DecisionRecordID, action.TradeID, 
	action.Action, action.Symbol, action.Quantity, action.Price, action.Leverage,
	action.OrderID, action.Timestamp, action.Success, action.ErrorMessage)
	
	return err
}

// GetTradeActions 获取交易动作记录
func (d *Database) GetTradeActions(traderID string, limit int) ([]*TradeActionRecord, error) {
	query := `
		SELECT id, trader_id, decision_record_id, trade_id, action, symbol,
			quantity, price, leverage, order_id, timestamp, success, error_message, created_at
		FROM trade_actions 
		WHERE trader_id = ? 
		ORDER BY timestamp DESC
	`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	
	rows, err := d.db.Query(query, traderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var actions []*TradeActionRecord
	for rows.Next() {
		var action TradeActionRecord
		var decisionID, tradeID sql.NullString
		
		err := rows.Scan(
			&action.ID, &action.TraderID, &decisionID, &tradeID, &action.Action,
			&action.Symbol, &action.Quantity, &action.Price, &action.Leverage,
			&action.OrderID, &action.Timestamp, &action.Success, &action.ErrorMessage,
			&action.CreatedAt)
		
		if err != nil {
			continue
		}
		
		if decisionID.Valid {
			action.DecisionRecordID = &decisionID.String
		}
		if tradeID.Valid {
			action.TradeID = &tradeID.String
		}
		
		actions = append(actions, &action)
	}
	
	return actions, nil
}

// CreateDecisionRecord 创建决策记录
func (d *Database) CreateDecisionRecord(record *DecisionRecordDB) error {
	if record.ID == "" {
		record.ID = fmt.Sprintf("decision_%d", time.Now().UnixNano())
	}
	
	_, err := d.db.Exec(`
		INSERT INTO decision_records (
			id, trader_id, cycle_number, timestamp, system_prompt, input_prompt,
			cot_trace, decision_json, account_state_json, positions_json,
			candidate_coins_json, execution_log_json, success, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ID, record.TraderID, record.CycleNumber, record.Timestamp,
	record.SystemPrompt, record.InputPrompt, record.CoTTrace, record.DecisionJSON,
	record.AccountStateJSON, record.PositionsJSON, record.CandidateCoinsJSON,
	record.ExecutionLogJSON, record.Success, record.ErrorMessage)
	
	return err
}

// GetTradeStatistics 获取交易统计信息
func (d *Database) GetTradeStatistics(traderID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// 总交易数
	var totalTrades int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM trades WHERE trader_id = ? AND status != 'open'`, traderID).Scan(&totalTrades)
	if err != nil {
		return nil, err
	}
	stats["total_trades"] = totalTrades
	
	// 盈利交易数
	var winningTrades int
	err = d.db.QueryRow(`SELECT COUNT(*) FROM trades WHERE trader_id = ? AND status != 'open' AND pnl > 0`, traderID).Scan(&winningTrades)
	if err != nil {
		return nil, err
	}
	stats["winning_trades"] = winningTrades
	
	// 亏损交易数
	var losingTrades int
	err = d.db.QueryRow(`SELECT COUNT(*) FROM trades WHERE trader_id = ? AND status != 'open' AND pnl < 0`, traderID).Scan(&losingTrades)
	if err != nil {
		return nil, err
	}
	stats["losing_trades"] = losingTrades
	
	// 胜率
	if totalTrades > 0 {
		stats["win_rate"] = float64(winningTrades) / float64(totalTrades) * 100
	} else {
		stats["win_rate"] = 0.0
	}
	
	// 总盈亏
	var totalPnL sql.NullFloat64
	err = d.db.QueryRow(`SELECT SUM(pnl) FROM trades WHERE trader_id = ? AND status != 'open'`, traderID).Scan(&totalPnL)
	if err != nil {
		return nil, err
	}
	if totalPnL.Valid {
		stats["total_pnl"] = totalPnL.Float64
	} else {
		stats["total_pnl"] = 0.0
	}
	
	// 平均盈利
	var avgWin sql.NullFloat64
	err = d.db.QueryRow(`SELECT AVG(pnl) FROM trades WHERE trader_id = ? AND status != 'open' AND pnl > 0`, traderID).Scan(&avgWin)
	if err != nil {
		return nil, err
	}
	if avgWin.Valid {
		stats["avg_win"] = avgWin.Float64
	} else {
		stats["avg_win"] = 0.0
	}
	
	// 平均亏损
	var avgLoss sql.NullFloat64
	err = d.db.QueryRow(`SELECT AVG(pnl) FROM trades WHERE trader_id = ? AND status != 'open' AND pnl < 0`, traderID).Scan(&avgLoss)
	if err != nil {
		return nil, err
	}
	if avgLoss.Valid {
		stats["avg_loss"] = avgLoss.Float64
	} else {
		stats["avg_loss"] = 0.0
	}
	
	return stats, nil
}

// GetTradePerformanceAnalysis 获取交易表现分析（替代文件式AnalyzePerformance）
func (d *Database) GetTradePerformanceAnalysis(traderID string, limit int) (map[string]interface{}, error) {
	// 获取最近的交易记录
	trades, err := d.GetTraderTrades(traderID, limit)
	if err != nil {
		return nil, err
	}
	
	// 初始化分析结果，保持与原PerformanceAnalysis结构一致
	analysis := map[string]interface{}{
		"total_trades":   0,
		"winning_trades": 0,
		"losing_trades":  0,
		"win_rate":       0.0,
		"avg_win":        0.0,
		"avg_loss":       0.0,
		"profit_factor":  0.0,
		"sharpe_ratio":   0.0,
		"recent_trades":  []map[string]interface{}{},
		"symbol_stats":   map[string]interface{}{},
		"best_symbol":    "",
		"worst_symbol":   "",
	}
	
	// 如果没有交易记录
	if len(trades) == 0 {
		return analysis, nil
	}
	
	// 筛选已完成的交易
	var completedTrades []*TradeRecord
	for _, trade := range trades {
		if trade.Status == "closed" && trade.ClosePrice != nil && trade.CloseTime != nil {
			completedTrades = append(completedTrades, trade)
		}
	}
	
	if len(completedTrades) == 0 {
		return analysis, nil
	}
	
	// 统计基础数据
	totalTrades := len(completedTrades)
	winningTrades := 0
	losingTrades := 0
	totalWinAmount := 0.0  // 总盈利金额
	totalLossAmount := 0.0 // 总亏损金额（负数）
	
	// 各币种统计
	symbolStats := make(map[string]map[string]interface{})
	
	// 转换为recent_trades格式，同时进行统计
	var recentTrades []map[string]interface{}
	for _, trade := range completedTrades {
		// 转换为前端期望的格式
		tradeMap := map[string]interface{}{
			"symbol":         trade.Symbol,
			"side":           trade.Side,
			"quantity":       trade.Quantity,
			"leverage":       trade.Leverage,
			"open_price":     trade.OpenPrice,
			"close_price":    *trade.ClosePrice,
			"position_value": trade.PositionValue,
			"margin_used":    trade.MarginUsed,
			"pn_l":          trade.PnL,
			"pn_l_pct":      trade.PnLPct,
			"duration":      formatTradeDuration(trade.DurationSecs),
			"open_time":     trade.OpenTime,
			"close_time":    *trade.CloseTime,
			"was_stop_loss": trade.CloseReason == "stop_loss",
		}
		recentTrades = append(recentTrades, tradeMap)
		
		// 统计盈亏
		if trade.PnL > 0 {
			winningTrades++
			totalWinAmount += trade.PnL
		} else if trade.PnL < 0 {
			losingTrades++
			totalLossAmount += trade.PnL // 累加负数
		}
		
		// 各币种统计
		symbol := trade.Symbol
		if _, exists := symbolStats[symbol]; !exists {
			symbolStats[symbol] = map[string]interface{}{
				"symbol":         symbol,
				"total_trades":   0,
				"winning_trades": 0,
				"losing_trades":  0,
				"win_rate":       0.0,
				"total_pn_l":     0.0,
				"avg_pn_l":       0.0,
			}
		}
		
		stats := symbolStats[symbol]
		stats["total_trades"] = stats["total_trades"].(int) + 1
		stats["total_pn_l"] = stats["total_pn_l"].(float64) + trade.PnL
		
		if trade.PnL > 0 {
			stats["winning_trades"] = stats["winning_trades"].(int) + 1
		} else if trade.PnL < 0 {
			stats["losing_trades"] = stats["losing_trades"].(int) + 1
		}
	}
	
	// 计算统计指标（完全按照原逻辑）
	analysis["total_trades"] = totalTrades
	analysis["winning_trades"] = winningTrades
	analysis["losing_trades"] = losingTrades
	
	if totalTrades > 0 {
		analysis["win_rate"] = (float64(winningTrades) / float64(totalTrades)) * 100
		
		// 计算平均盈利和平均亏损
		if winningTrades > 0 {
			analysis["avg_win"] = totalWinAmount / float64(winningTrades)
		} else {
			analysis["avg_win"] = 0.0
		}
		if losingTrades > 0 {
			analysis["avg_loss"] = totalLossAmount / float64(losingTrades)
		} else {
			analysis["avg_loss"] = 0.0
		}
		
		// Profit Factor = 总盈利 / 总亏损（绝对值）
		// 注意：totalLossAmount 是负数，所以取负号得到绝对值
		if totalLossAmount != 0 {
			analysis["profit_factor"] = totalWinAmount / (-totalLossAmount)
		} else if totalWinAmount > 0 {
			// 只有盈利没有亏损的情况，设置为一个很大的值表示完美策略
			analysis["profit_factor"] = 999.0
		} else {
			analysis["profit_factor"] = 0.0
		}
	}
	
	// 计算各币种胜率和平均盈亏，找出最好和最差币种
	bestPnL := -999999.0
	worstPnL := 999999.0
	bestSymbol := ""
	worstSymbol := ""
	
	for symbol, stats := range symbolStats {
		totalSymbolTrades := stats["total_trades"].(int)
		if totalSymbolTrades > 0 {
			winningSymbolTrades := stats["winning_trades"].(int)
			totalSymbolPnL := stats["total_pn_l"].(float64)
			
			stats["win_rate"] = (float64(winningSymbolTrades) / float64(totalSymbolTrades)) * 100
			stats["avg_pn_l"] = totalSymbolPnL / float64(totalSymbolTrades)
			
			if totalSymbolPnL > bestPnL {
				bestPnL = totalSymbolPnL
				bestSymbol = symbol
			}
			if totalSymbolPnL < worstPnL {
				worstPnL = totalSymbolPnL
				worstSymbol = symbol
			}
		}
	}
	
	analysis["symbol_stats"] = symbolStats
	analysis["best_symbol"] = bestSymbol
	analysis["worst_symbol"] = worstSymbol
	
	// 只保留最近的交易（倒序：最新的在前）
	if len(recentTrades) > 10 {
		// 反转数组，让最新的在前
		for i, j := 0, len(recentTrades)-1; i < j; i, j = i+1, j-1 {
			recentTrades[i], recentTrades[j] = recentTrades[j], recentTrades[i]
		}
		recentTrades = recentTrades[:10]
	} else if len(recentTrades) > 0 {
		// 反转数组
		for i, j := 0, len(recentTrades)-1; i < j; i, j = i+1, j-1 {
			recentTrades[i], recentTrades[j] = recentTrades[j], recentTrades[i]
		}
	}
	
	analysis["recent_trades"] = recentTrades
	
	// 计算夏普比率（基于单笔交易的盈亏百分比）
	sharpeRatio := d.calculateSharpeRatioFromTrades(completedTrades)
	analysis["sharpe_ratio"] = sharpeRatio
	
	return analysis, nil
}

// formatTradeDuration 格式化交易持续时间为用户友好的显示格式
func formatTradeDuration(durationSecs int) string {
	if durationSecs < 60 {
		return fmt.Sprintf("%d秒", durationSecs)
	} else if durationSecs < 3600 {
		minutes := durationSecs / 60
		seconds := durationSecs % 60
		if seconds == 0 {
			return fmt.Sprintf("%d分", minutes)
		}
		return fmt.Sprintf("%d分%d秒", minutes, seconds)
	} else if durationSecs < 86400 {
		hours := durationSecs / 3600
		minutes := (durationSecs % 3600) / 60
		if minutes == 0 {
			return fmt.Sprintf("%d小时", hours)
		}
		return fmt.Sprintf("%d小时%d分", hours, minutes)
	} else {
		days := durationSecs / 86400
		hours := (durationSecs % 86400) / 3600
		if hours == 0 {
			return fmt.Sprintf("%d天", days)
		}
		return fmt.Sprintf("%d天%d小时", days, hours)
	}
}

// calculateSharpeRatioFromTrades 基���交易记录计算夏普比率
func (d *Database) calculateSharpeRatioFromTrades(trades []*TradeRecord) float64 {
	if len(trades) < 2 {
		return 0.0
	}
	
	// 提取每笔交易的盈亏百分比作为周期收益率
	var returns []float64
	for _, trade := range trades {
		if trade.MarginUsed > 0 {
			// 将盈亏百分比转换为小数形式（例如：5% -> 0.05）
			periodReturn := trade.PnLPct / 100.0
			returns = append(returns, periodReturn)
		}
	}
	
	if len(returns) < 2 {
		return 0.0
	}
	
	// 计算平均收益率
	sumReturns := 0.0
	for _, r := range returns {
		sumReturns += r
	}
	meanReturn := sumReturns / float64(len(returns))
	
	// 计算收益率标准差
	sumSquaredDiff := 0.0
	for _, r := range returns {
		diff := r - meanReturn
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(returns))
	stdDev := math.Sqrt(variance)
	
	// 避免除以零
	if stdDev == 0 {
		if meanReturn > 0 {
			return 999.0 // 无波动的正收益
		} else if meanReturn < 0 {
			return -999.0 // 无波动的负收益
		}
		return 0.0
	}
	
	// 计算夏普比率（假设无风险利率为0）
	// 注：直接返回周期级别的夏普比率（非年化），正常范围 -2 到 +2
	sharpeRatio := meanReturn / stdDev
	return sharpeRatio
}
