package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tel "1C/Telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"golang.org/x/net/proxy"

	"github.com/sirupsen/logrus"
)

const (
	BotToken = "735761544:AAEXq6FKx9B_-CHY7WyshpmO0Zb8LWFikFQ"
)

type Hook struct {
}

func (h *Hook) Levels() []logrus.Level {
	return []logrus.Level{logrus.ErrorLevel, logrus.PanicLevel}
}
func (h *Hook) Fire(En *logrus.Entry) error {
	fmt.Println(En.Message)
	return nil
}

/* type settings struct {
	BinPath       string                          `json:"BinPath"`
	Extensions    *cf.ExtensionsSettings          `json:"Extensions"`
	Configuration *cf.ConfigurationCommonSettings `json:"Configuration"`
} */

var (
	//confFile string
	pass     string
	LogLevel int
	TempFile string
)

func main() {
	Tasks := new(tel.Tasks)
	Tasks.ReadSettings()

	defer inilogrus().Stop()
	defer DeleleEmptyFile(logrus.StandardLogger().Out.(*os.File))

	if pass != "" {
		Tasks.SetPass(pass)
		fmt.Println("Пароль установлен")
		return
	}

	bot := NewBotAPI()
	if bot == nil {
		logrus.Panic("Не удалось подключить бота")
		return
	} else {
		logrus.Debug("К боту подключились")
	}

	/* info, _ := bot.GetWebhookInfo()
	fmt.Println(info) */

	http.HandleFunc("/Debug", func(w http.ResponseWriter, r *http.Request) {
		ioutil.ReadAll(r.Body)
		defer r.Body.Close()

		w.Write([]byte("Конект есть"))
		//fmt.Println("Конект есть")
	})

	updates := bot.ListenForWebhook("/")
	if net := tel.Confs.Network; net != nil {
		go http.ListenAndServe(":"+net.ListenPort, nil)
		//go http.ListenAndServeTLS(":"+net.ListenPort, "webhook_cert.pem", "webhook_pkey.key", nil)
		logrus.Info("Слушаем порт " + net.ListenPort)
	} else {
		logrus.Panic("В настройках не определен параметр ListenPort")
		return
	}

	// получаем все обновления из канала updates
	for update := range updates {
		var Command string

		if update.Message != nil {
			if ok, comment := Tasks.Authentication(update.Message.Text); !ok {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Необходимо ввести пароль"))
				continue
			} else {
				if comment != "" {
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, comment+"\nДля начала работы воспользуйтесь командой /start"))
					continue
				}
			}
		}

		//	fmt.Println(update.Message.Text)
		if update.CallbackQuery != nil {
			existNew := false
			for _, t := range Tasks.GetTasks(update.CallbackQuery.From.ID) {
				if t.GetState() == tel.StateNew {
					callback := t.GetCallBack()
					call := callback[update.CallbackQuery.Data]
					if call != nil {
						call(update.CallbackQuery.Data)
					}
					existNew = true
				}
			}
			if !existNew {
				bot.Send(tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Не найдено активных заданий, выполните /start"))
			}
			//b.callback[update.CallbackQuery.Data](func() { bot.Send(tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Выполнено")) })
			//fmt.Println(update.CallbackQuery.Message.Chat.ID)
			//bot.Send(tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Ok, I remember"))
			continue
		}

		if update.Message == nil {
			logrus.Debug("Message = nil")
			continue
		}

		Command = update.Message.Command()
		logrus.WithFields(logrus.Fields{
			"Command":   Command,
			"Msg":       update.Message.Text,
			"FirstName": update.Message.From.FirstName,
			"LastName":  update.Message.From.LastName,
			"UserName":  update.Message.From.UserName,
		}).Debug()

		switch Command {
		case "start":
			Tasks.Reset(bot, &update, false)
		case "BuildCf":
			//fmt.Println(update.Message.Chat.ID)
			name := "BuildCf"
			task := new(tel.BuildCf)
			task.Ini(name)

			fromID := update.Message.From.ID
			if err := Tasks.Append(task, fromID); err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, err.Error()))
			} else {
				task.StartInitialise(bot, &update, func() { Tasks.Delete(fromID) })
			}
		case "BuildCfe":
			name := "BuildCf"
			task := new(tel.BuildCfe)
			task.Ini(name)

			fromID := update.Message.From.ID
			if err := Tasks.Append(task, fromID); err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, err.Error()))
			} else {
				task.StartInitialise(bot, &update, func() { Tasks.Delete(fromID) })
			}
		case "BuilAndUploadCf":
			name := "BuilAndUploadCf"
			task := new(tel.BuilAndUploadCf)
			task.Ini(name)

			fromID := update.Message.From.ID
			if err := Tasks.Append(task, fromID); err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, err.Error()))
			} else {
				task.StartInitialiseDesc(bot, &update, func() { Tasks.Delete(fromID) })
			}
		case "BuilAndUploadCfe":
			name := "BuilAndUploadCfe"
			task := new(tel.BuilAndUploadCfe)
			task.Ini(name)

			fromID := update.Message.From.ID
			if err := Tasks.Append(task, fromID); err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, err.Error()))
			} else {
				task.StartInitialiseDesc(bot, &update, func() { Tasks.Delete(fromID) })
			}
		case "GetListUpdateState":
			name := "GetListUpdateState"
			task := new(tel.GetListUpdateState)
			task.Ini(name)

			fromID := update.Message.From.ID
			if err := Tasks.Append(task, fromID); err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, err.Error()))
			} else {
				task.StartInitialise(bot, &update, func() { Tasks.Delete(fromID) })
			}
		case "SetPlanUpdate":
			name := "SetPlanUpdate"
			task := new(tel.SetPlanUpdate)
			task.Ini(name)

			fromID := update.Message.From.ID
			if err := Tasks.Append(task, fromID); err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, err.Error()))
			} else {
				task.StartInitialise(bot, &update, func() { Tasks.Delete(fromID) })
			}
		case "Cancel":
			Tasks.Reset(bot, &update, true)
		default:
			// Проверяем общие хуки
			if Tasks.ExecuteHook(&update, update.Message.From.ID) {
				continue
			} else {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Я такому необученный, выполните /start"))
			}
			//bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Простите, такого я не умею"))
		}

		//	context = new(Context)

		/* 	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Что прикажите?")

		keyboard := tgbotapi.InlineKeyboardMarkup{}
		var row []tgbotapi.InlineKeyboardButton
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Запланировать обновление конфигурации", "ваааа")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Запланировать обновление расширения", "4354")
		row = append(row, []tgbotapi.InlineKeyboardButton{btn1, btn2}...)
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)

		msg.ReplyMarkup = &keyboard
		bot.Send(msg)
		*/
		/* M := tgbotapi.NewMessage(
			update.Message.Chat.ID,
			"Привет",
		)
		bot.Send(M) */

	}
	/* 	return

	   	if S.BinPath == "" {
	   		logrus.Error("В конфигурационном файле не определен параметр BinPath")
	   		return
	   	} */

	/* S = new(settings)
	S.Extensions = new(ExtensionsSettings)
	S.Extensions.FreshConf = new(FreshConf)

	S.Configuration = new(cf.ConfigurationCommonSettings)
	S.Configuration.Confs = []*cf.ConfigurationSettings{&cf.ConfigurationSettings{}}
	S.Configuration.Confs[0].RConf = new(cf.RepositoryConf)

	b, _ := json.Marshal(S)
	fmt.Println(string(b)) */
	/*
		wg := new(sync.WaitGroup)

		if S.Configuration != nil {
			wg.Add(1)
			go BuildAndSendCF(wg, S.BinPath, S.Configuration)
		}

		if S.Extensions != nil {
			wg.Add(1)
			go BuildAndSendEx(wg, S.BinPath, S.Extensions)
		}
		wg.Wait() */
}

/*
func BuildAndSendCF(wg *sync.WaitGroup, BinPath string, Configuration *cf.ConfigurationCommonSettings) {
	 defer wg.Done()

	conf := new(cf.ConfCommonData)
	conf.BinPath = BinPath

	wgSend := new(sync.WaitGroup)
	ChConf := make(chan *cf.ConfigurationSettings, len(Configuration.Confs))

	if Configuration.FreshConf != nil {
		for range Configuration.Confs {
			wgSend.Add(1)
			go SendConfToFresh(wgSend, ChConf, Configuration.FreshConf)
		}
	}
	conf.SaveConfiguration(ChConf, Configuration)
	close(ChConf)
	wgSend.Wait()
}


func BuildAndSendEx(wg *sync.WaitGroup, BinPath string, Extensions *cf.ExtensionsSettings) {
	 defer wg.Done()

	conf := new(cf.ConfCommonData)
	conf.BinPath = BinPath

	wgSend := new(sync.WaitGroup)
	pool := 5
	chExt := make(chan string, pool)

	if Extensions.FreshConf != nil {
		for i := 0; i < pool; i++ {
			wgSend.Add(1)
			go SendExToFresh(wgSend, chExt, Extensions.FreshConf)
		}
	}
	conf.BuildExtensions(chExt, Extensions)
	close(chExt)
	wgSend.Wait()
}

func SendConfToFresh(wg *sync.WaitGroup, cfConf <-chan *cf.ConfigurationSettings, FreshConf *fresh.FreshConf) {
	defer wg.Done()

	wgLock := new(sync.WaitGroup)
	for c := range cfConf {
		wgLock.Add(1)
		fresh := new(fresh.Fresh)
		fresh.Conf = FreshConf
		fresh.ConfCode = c.RConf.ConfFreshName
		fresh.ConfComment = fmt.Sprintf("Автосборка. Конфигурация версии %v", c.RConf.Revision)

		go fresh.RegConfigurations(wgLock, c.CfName)
	}
	wgLock.Wait()
}
*/

/*
func SendExToFresh(wg *sync.WaitGroup, exName <-chan string, FreshConf *fresh.FreshConf) {
	defer wg.Done()

	wgLock := new(sync.WaitGroup)
	for c := range exName {
		wgLock.Add(1)
		fresh := new(fresh.Fresh)
		fresh.Conf = FreshConf

		//deleleF := func() {
		//	if Extensions.DeleteFile {
		//		logrus.WithField("Файл", filename).Debug("Удаляем файл")
		//		os.RemoveAll(filename)
		//	}
		//}
		go fresh.RegExtension(wgLock, c, nil)
	}
	wgLock.Wait()
}
*/

func getFiles(rootDir, ext string) []string {
	var result []string
	f := func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && filepath.Ext(info.Name()) == ext {
			result = append(result, path)
		}

		return nil
	}

	filepath.Walk(rootDir, f)
	return result
}

func NewBotAPI() *tgbotapi.BotAPI {
	// create a socks5 dialer

	httpClient := new(http.Client)
	if net := tel.Confs.Network; net != nil {
		logrus.Debug("Используем прокси " + net.PROXY_ADDR)

		dialer, err := proxy.SOCKS5("tcp", net.PROXY_ADDR, nil, proxy.Direct)
		if err != nil {
			logrus.WithField("Прокси", net.PROXY_ADDR).Errorf("Ошибка соединения с прокси: %q", err)
			return nil
		}
		// setup a http client
		httpTransport := &http.Transport{}
		httpTransport.Dial = dialer.Dial
		httpClient = &http.Client{Transport: httpTransport}
	}

	bot, err := tgbotapi.NewBotAPIWithClient(BotToken, httpClient)
	if err != nil {
		logrus.Errorf("Произошла ошибка при создании бота: %q", err)
		return nil
	}

	if net := tel.Confs.Network; net != nil {
		logrus.Debug("Устанавливаем хук на URL " + net.WebhookURL)
		/* CertPath := "Cert.pfx"
		if _, err := os.Stat(CertPath); os.IsNotExist(err) {
			logrus.WithField("файл", CertPath).Panic("Не найден сертификат")
		}

		//file, err := os.Open(CertPath)
		//defer file.Close()
		bytes, err := ioutil.ReadFile(CertPath)
		if err != nil {
			logrus.WithField("файл", CertPath).Panicf("Не удалось открыть сертификат: %v", err)
		} */

		//_, err = bot.SetWebhook(tgbotapi.NewWebhookWithCert(net.WebhookURL, "webhook_cert.pem"))
		_, err = bot.SetWebhook(tgbotapi.NewWebhook(net.WebhookURL))
		if err != nil {
			logrus.Errorf("Произошла ошибка при установки веб хука для бота: %q", err)
			return nil
		}
	} else {
		logrus.Panic("В настройках не определен параметр WebhookURL")
		return nil
	}

	//bot.Debug = true
	return bot
}

func inilogrus() *time.Ticker {
	//flag.StringVar(&confFile, "conffile", "", "Конфигурационный файл")
	flag.StringVar(&pass, "SetPass", "", "Установка нового пвроля")
	flag.IntVar(&LogLevel, "LogLevel", 3, "Уровень логирования от 2 до 5, где 2 - ошибка, 3 - предупреждение, 4 - информация, 5 - дебаг")

	flag.Parse()

	currentDir, _ := os.Getwd()

	var LogDir string
	if dir := tel.Confs.LogDir; dir != "" {
		LogDir = tel.Confs.LogDir
		LogDir = strings.Replace(LogDir, "%AppDir%", currentDir, -1)
		if _, err := os.Stat(LogDir); os.IsNotExist(err) {
			os.Mkdir(LogDir, os.ModePerm)
		}
	} else {
		LogDir = currentDir
	}

	Log, _ := os.OpenFile(filepath.Join(LogDir, "Log_"+time.Now().Format("02.01.2006 15.04.05")), os.O_CREATE, os.ModeAppend)
	logrus.SetOutput(Log)

	timer := time.NewTicker(time.Minute * 10)
	go func() {
		for range timer.C {
			Log, _ := os.OpenFile(filepath.Join(LogDir, "Log_"+time.Now().Format("02.01.2006 15.04.05")), os.O_CREATE, os.ModeAppend)
			oldFile := logrus.StandardLogger().Out.(*os.File)
			logrus.SetOutput(Log)
			DeleleEmptyFile(oldFile)
		}
	}()

	logrus.SetLevel(logrus.Level(LogLevel))
	logrus.AddHook(new(Hook))

	//line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	//fmt.Println(line)

	return timer
}

func DeleleEmptyFile(file *os.File) {
	// Если файл пустой, удаляем его. что бы не плодил кучу файлов
	info, _ := file.Stat()
	if info.Size() == 0 {
		file.Close()

		if err := os.Remove(file.Name()); err != nil {
			logrus.WithError(err).WithField("Файл", file.Name()).Error("Ошибка удаления пустого файла логов")
		}
	}
}
