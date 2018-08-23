package config

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	_ "github.com/go-sql-driver/mysql" // 初始化 Mysql 驱动
	"github.com/go-xorm/core"
	"github.com/go-xorm/xorm"
	"github.com/gorilla/securecookie"
	jwtmiddleware "github.com/iris-contrib/middleware/jwt"
	"github.com/kataras/iris"
	"github.com/kataras/iris/middleware/pprof"
	"github.com/kataras/iris/middleware/recover"
	"github.com/kataras/iris/mvc"
	"github.com/kataras/iris/sessions"
	"github.com/kataras/iris/sessions/sessiondb/boltdb"
	"github.com/kataras/iris/view"
	"gopkg.in/yaml.v2"
)

var app *iris.Application
var XOrmEngine *xorm.Engine // XOrmEngine 全局 Xorm 引擎对象
var databaseYml = "resources/configs/database.yml"
var applicationYml = "resources/configs/application.yml"
var sess *sessions.Sessions
var ApplicationConfig *Application // ApplicationConfig 全局配置文件对象
var sessionInitSync sync.Once

func initDatabase() {
	dbconfig := new(DatabaseConfig)
	//解析数据库配置项
	parseConfig(databaseYml, dbconfig)
	dsn := dbconfig.Mysql.DbUser + ":" +
		dbconfig.Mysql.DbPassword + "@tcp(" +
		dbconfig.Mysql.DbServer + ":" +
		dbconfig.Mysql.DbPort + ")/" +
		dbconfig.Mysql.DbName + "?charset=" +
		dbconfig.Mysql.DbChatSet
	_orm, err := xorm.NewEngine("mysql", dsn)
	if err != nil {
		panic(err.Error())
	}
	err = _orm.Ping() //检测是否联通数据库
	if err != nil {
		panic(err.Error())
	}
	XOrmEngine = _orm
	XOrmEngine.Logger().SetLevel(core.LOG_ERR)
	XOrmEngine.ShowSQL(dbconfig.Orm.ShowSql)
	XOrmEngine.ShowExecTime(dbconfig.Orm.ShowExecTime)
	XOrmEngine.SetMaxOpenConns(int(dbconfig.Orm.MaxOpenConns))
	XOrmEngine.SetMaxIdleConns(int(dbconfig.Orm.MaxIdleConns))
}

func parseConfig(path string, out interface{}) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		panic(err.Error())
	}
	fileContent, err := ioutil.ReadFile(absPath)
	if err != nil {
		panic(err.Error())
	}
	err = yaml.Unmarshal(fileContent, out)
	if err != nil {
		panic(err.Error())
	}
}

func StartApplication() {
	//初始化数据库ORM
	initDatabase()
	ApplicationConfig = new(Application)
	parseConfig(applicationYml, ApplicationConfig) //解析配置
	//实例化服务器
	app = iris.New()
	//配置PPROF
	if ApplicationConfig.Pprof.Open {
		app.Get(ApplicationConfig.Pprof.Route, pprof.New())
	}
	var viewEngine int64 = 0
	//附加视图
	if ApplicationConfig.View.Engine.Django.Path != "" && ApplicationConfig.View.Engine.Django.Suffix != "" {
		viewEngine = viewEngine + 1
		app.RegisterView(
			view.Django(
				ApplicationConfig.View.Engine.Django.Path,
				ApplicationConfig.View.Engine.Django.Suffix,
			).Reload(ApplicationConfig.View.Reload),
		) //不缓存模板
	}
	if ApplicationConfig.View.Engine.Html.Path != "" && ApplicationConfig.View.Engine.Html.Suffix != "" {
		viewEngine = viewEngine + 1
		app.RegisterView(view.HTML(
			ApplicationConfig.View.Engine.Html.Path,
			ApplicationConfig.View.Engine.Html.Suffix,
		).Reload(ApplicationConfig.View.Reload))

	}
	if viewEngine == 0 {
		panic("请至少配置一个模板引擎")
	}
	//注册静态资源路由
	registerStatic()
	//配置recover插件
	app.Use(recover.New())
	//日志
	app.Use(jwtmiddleware.New(jwtmiddleware.Config{
		ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
			return []byte("My Secret"), nil
		},
		// When set, the middleware verifies that tokens are signed with the specific signing algorithm
		// If the signing method is not constant the ValidationKeyGetter callback can be used to implement additional checks
		// Important to avoid security issues described here: https://auth0.com/blog/2015/03/31/critical-vulnerabilities-in-json-web-token-libraries/
		SigningMethod: jwt.SigningMethodHS256,
	}).Serve)

	//app.Use(logger.New())
	//注册错误路由
	registerErrorRoutes()
	//注册后端路由
	registerBackendRoutes()
	////注册前端路由
	registerFrontendRoutes()
	//构建并且运行应用
	runServe(ApplicationConfig)
}

func registerStatic() {
	app.StaticWeb("/upload", filepath.FromSlash("./resources/assets/upload"))
	app.StaticWeb("/frontend", filepath.FromSlash("./resources/assets/frontend"))
	app.StaticWeb("/backend", filepath.FromSlash("./resources/assets/backend"))
	app.StaticWeb("/resume", filepath.FromSlash("./resources/assets/resume"))
}

func runServe(config *Application) {
	if config.Pprof.Open {
		go func() {
			pport := strconv.Itoa(int(config.Pprof.Port))
			err := http.ListenAndServe("0.0.0.0:"+pport, nil)
			if err != nil {
				log.Println(err.Error())
			}
		}()
	}
	port := strconv.Itoa(int(config.Port))
	app.Run(iris.Addr(":"+port), iris.WithCharset(config.Charset))
}

// BaseMvc 构造 mvc基础,分离相关session
func BaseMvc(config *Application) func(app *mvc.Application) {
	sessionInitSync.Do(func() {
		hashKey := []byte(config.HashKey)
		blockKey := []byte(config.BlockKey)
		secureCookie := securecookie.New(hashKey, blockKey)
		sess = sessions.New(sessions.Config{
			Cookie:  config.Session.Name,
			Encode:  secureCookie.Encode,
			Decode:  secureCookie.Decode,
			Expires: config.Session.Expires * time.Second,
		})
		db, err := boltdb.New("./runtime/sessions.db", os.FileMode(0750)) //优化性能, 如果分离前后端session 会使内存使用量增加一倍.
		if err != nil {
			panic(err)
		}
		// close and unlobkc the database when control+C/cmd+C pressed
		iris.RegisterOnInterrupt(func() {
			db.Close()
		})
		sess.UseDatabase(db)
	})
	return func(app *mvc.Application) {
		app.Register(sess.Start, XOrmEngine)
	}
}
