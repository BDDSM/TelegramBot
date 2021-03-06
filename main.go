package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	session "TelegramBot/Confs"
	n "TelegramBot/Net"
	tel "TelegramBot/TelegramTasks"
	logrusRotate "github.com/LazarenkoA/LogrusRotate"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"

	"github.com/sirupsen/logrus"
)


type ngrokAPI struct {
	Tunnels []*struct {
		PublicUrl string `json:"public_url"`
	} `json:"tunnels"`
}
type RotateConf struct {
}
type Hook struct {
}

func (h *Hook) Levels() []logrus.Level {
	return []logrus.Level{logrus.ErrorLevel, logrus.PanicLevel}
}
func (h *Hook) Fire(en *logrus.Entry) error {
	log.Println(en.Message)
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
	//TempFile  string

)

func main() {
	var err error

	flag.StringVar(&pass, "SetPass", "", "Установка нового пвроля")
	flag.IntVar(&LogLevel, "LogLevel", 3, "Уровень логирования от 2 до 5, где 2 - ошибка, 3 - предупреждение, 4 - информация, 5 - дебаг")
	flag.Parse()
	logrus.SetLevel(logrus.Level(2))
	logrus.AddHook(new(Hook))

	fmt.Printf("%-50v", "Читаем настройки")
	Tasks := new(tel.Tasks)
	if err = Tasks.ReadSettings(); err == nil {
		fmt.Println("ОК")
	} else {
		fmt.Println("FAIL")
		logrus.Errorf("%v", err)
		return
	}
	fmt.Printf("%-50v", "Уровень логирования")
	fmt.Println(LogLevel)


	lw := new(logrusRotate.Rotate).Construct()
	defer lw.Start(LogLevel, new(RotateConf))()

	fmt.Printf("%-50v", "Подключаемся к redis")
	if Tasks.SessManager, err = session.NewSessionManager(); err == nil {
		fmt.Println("ОК")
	} else {
		fmt.Println("FAIL")
		logrus.Errorf("%v", err)
		return
	}

	if pass != "" {
		Tasks.SetPass(pass)
		fmt.Println("Пароль установлен")
		return
	}

	fmt.Printf("%-50v", "Получаем настройки ngrok")
	var WebhookURL string
	if WebhookURL, err = getNgrokURL(); err == nil {
		fmt.Println("ОК")
	} else {
		fmt.Println("FAIL")
		logrus.Errorf("%v", err)
		return
	}

	fmt.Printf("%-50v", "Создаем бота")
	bot := NewBotAPI(WebhookURL)
	if bot == nil {
		logrus.Panic("Не удалось подключить бота")
		return
	}
	logrus.Debug("К боту подключились")
	fmt.Println("ОК")

	/* info, _ := bot.GetWebhookInfo()
	fmt.Println(info) */

	http.HandleFunc("/Debug", func(w http.ResponseWriter, r *http.Request) {
		//ioutil.ReadAll(r.Body)
		//defer r.Body.Close()

		fmt.Fprintln(w, "Конект есть")
	})

	updates := bot.ListenForWebhook("/")
	if net := tel.Confs.Network; net != nil {
		go http.ListenAndServe(":"+net.ListenPort, nil)
		//go http.ListenAndServeTLS(":"+net.ListenPort, "webhook_cert.pem", "webhook_pkey.key", nil) // для SSL
		logrus.Info("Слушаем порт " + net.ListenPort)
	} else {
		logrus.Panic("В настройках не определен параметр ListenPort")
		return
	}

	fmt.Println("Бот запущен.")
	tf := new(tel.TaskFactory)
	mu := new(sync.Mutex) // некоторые задачи нельзя выполнять параллельно

	var imgMSG []tgbotapi.Message
	// получаем все обновления из канала updates
	for update := range updates {
		var Command string
		//update.Message.Photo[0].FileID
		//p := tgbotapi.NewPhotoShare(update.Message.Chat.ID, update.Message.Photo[0].FileID)
		//bot.GetFile(p)
		if update.Message != nil && ((update.Message.Command() != "" && update.Message.Command() != "start") || update.Message.Text != "") {
			if ok, comment := Tasks.CheckSession(update.Message.From, update.Message.Text); !ok {
				currentDir, _ := os.Getwd()
				imgPath := filepath.Join(currentDir, "img", "notLogin.jpg")

				if _, err := os.Stat(imgPath); os.IsNotExist(err) {
					m, _ := bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Необходимо ввести пароль \n"+comment))
					imgMSG = append(imgMSG, m)
				} else {
					// для отправки файла NewDocumentUpload
					msg := tgbotapi.NewPhotoUpload(update.Message.Chat.ID, imgPath)
					msg.Caption = "Вы кто такие? Я вас не звал, идите ...\n"
					m, _ := bot.Send(msg)
					imgMSG = append(imgMSG, m)
				}
				continue
			} else {
				if comment != "" {
					if update.Message != nil {
						bot.DeleteMessage(tgbotapi.DeleteMessageConfig{
							ChatID:    update.Message.Chat.ID,
							MessageID: update.Message.MessageID})
					}
					for _, m := range imgMSG {
						bot.DeleteMessage(tgbotapi.DeleteMessageConfig{
							ChatID:    m.Chat.ID,
							MessageID: m.MessageID})
					}
					imgMSG = []tgbotapi.Message{} // очистка
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "🧞‍♂ слушаюсь и повинуюсь."))
					continue
				}
			}
		}

		//	fmt.Println(update.Message.Text)
		if update.CallbackQuery != nil {
			existNew := false
			for _, t := range Tasks.GetTasks(update.CallbackQuery.From.ID) {
				if t.GetState() != tel.StateDone {
					callback := t.GetCallBack()
					call := callback[update.CallbackQuery.Data]
					if call != nil {
						call()
					}
					existNew = true
				}
			}
			if !existNew {
				bot.Send(tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, "Не найдено активных заданий."))
			}
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
			"ChatID":    update.Message.Chat.ID,
		}).Debug()

		fromID := update.Message.From.ID
		// Чистим старые задания
		Tasks.Delete(fromID)

		var task tel.ITask
		switch Command {
		case "start":
			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Привет %v %v!", update.Message.From.FirstName, update.Message.From.LastName)))
		case "buildcf":
			task = Tasks.AppendTask(tf.BuildCf(), Command, fromID, false)
		case "buildcfe":
			task = Tasks.AppendTask(tf.BuildCfe(), Command, fromID, false)
		//case "changeversion":
		//task = Tasks.AppendTask(new(tel.ChangeVersion), Command, fromID, false)
		case "buildanduploadcf":
			task = Tasks.AppendTask(tf.BuilAndUploadCf(), Command, fromID, false)
		case "buildanduploadcfe":
			task = Tasks.AppendTask(tf.BuilAndUploadCfe(), Command, fromID, false)
		case "getlistupdatestate":
			task = Tasks.AppendTask(tf.GetListUpdateState(), Command, fromID, true)
		case "setplanupdate":
			task = Tasks.AppendTask(tf.SetPlanUpdate(), Command, fromID, false)
		case "invokeupdate":
			task = Tasks.AppendTask(tf.IvokeUpdate(), Command, fromID, false)
		case "deployextension":
			task = Tasks.AppendTask(tf.DeployExtension(mu), Command, fromID, false)
		case "ivokeupdateactualcfe":
			task = Tasks.AppendTask(tf.IvokeUpdateActualCFE(), Command, fromID, false)
		case "disablezabbixmonitoring":
			task = Tasks.AppendTask(tf.DisableZabbixMonitoring(), Command, fromID, false)
		case "charts":
			task = Tasks.AppendTask(tf.Charts(), Command, fromID, false)
		case "cancel":
			//Tasks.Reset(fromID, bot, &update, true)
			//bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Готово!"))
		case "sendmsg":
			task = Tasks.AppendTask(tf.SendMsg(), Command, fromID, false)
		default:
			// Проверяем общие хуки
			if Tasks.ExecuteHook(&update) {
				continue
			}

			// обязательно асинхронно
			messageID := update.Message.MessageID
			message := update.Message
			go func() {
				var msg tgbotapi.MessageConfig
				if err := saveFile(message, bot); err != nil {
					msg = tgbotapi.NewMessage(message.Chat.ID, "Я такому необученный.")
					msg.ReplyToMessageID = messageID
				} else {
					msg = tgbotapi.NewMessage(message.Chat.ID, "👍🏻")
					msg.ReplyToMessageID = messageID
				}
				bot.Send(msg)
			}()
		}

		if task != nil {
			// горутина нужна из-за Lock
			go func() {
				task.Lock(func() {
					txt := fmt.Sprintf("Kоманда %q является эксклюзивной (параллельно несколько аналогичных команд выполняться не могут). Дождитесь окончания работы предыдущей команды", task.GetName())
					bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, txt))
				})

				// if race.Enabled {
				// 	bot.Send(tgbotapi.NewMessage(task.GetChatID(), fmt.Sprintf("Коданда %q является эксклюзивной.\n Дождитесь завершения работы предыдущей команды", task.GetName())))
				// }
				task.InfoWrapper(task.Initialise(bot, &update, func() {
					task.SetState(tel.StateDone)
					msg := tgbotapi.NewMessage(task.GetChatID(), fmt.Sprintf("Задание:\n%v\nГотово!", task.GetDescription()))
					msg.ReplyToMessageID = task.CurrentStep().GetMessageID()
					bot.Send(msg)

					Tasks.Delete(fromID)
					task.Unlock()
				}))
			}()
		}
	}
}

func getNgrokURL() (string, error) {
	if net := tel.Confs.Network; net != nil && net.UseNgrok {
		// файл Ngrok должен лежать рядом с основным файлом бота
		currentDir, _ := os.Getwd()
		ngrokpath := filepath.Join(currentDir, "ngrok.exe")
		//ngrokpath = "D:\\GoMy\\src\\TelegramBot\\ngrok.exe"
		if _, err := os.Stat(ngrokpath); os.IsNotExist(err) {
			return "", fmt.Errorf("Файл ngrok.exe не найден")
		}

		err := make(chan error, 0)
		result := make(chan string, 0)

		// горутина для запуска ngrok
		go func(chanErr chan<- error) {
			cmd := exec.Command(ngrokpath, "http", net.ListenPort)
			err := cmd.Run()
			if err != nil {
				errText := fmt.Sprintf("Произошла ошибка запуска:\n err:%v \n", err.Error())

				if cmd.Stderr != nil {
					if stderr := cmd.Stderr.(*bytes.Buffer).String(); stderr != "" {
						errText += fmt.Sprintf("StdErr:%v", stderr)
					}
				}
				chanErr <- fmt.Errorf(errText)
				close(chanErr)
			}
		}(err)

		// горутина для получения адреса
		go func(result chan<- string, chanErr chan<- error) {
			// задумка такая, в горутине выше стартует Ngrok, после запуска поднимается вебсервер на порту 4040
			// и я могу получать url через api. Однако, в текущей горутине я не знаю стартанут там Ngrok или нет, по этому таймер
			// продуем подключиться 5 раз (5 сек) если не получилось, ошибка.
			tryCount := 5
			timer := time.NewTicker(time.Second * 1)
			for range timer.C {
				resp, err := http.Get("http://localhost:4040/api/tunnels")
				if (err == nil && !(resp.StatusCode >= http.StatusOK && resp.StatusCode <= http.StatusIMUsed)) || err != nil {
					if tryCount--; tryCount <= 0 {
						chanErr <- fmt.Errorf("Не удалось получить данные ngrok")
						close(chanErr)
						timer.Stop()
						return
					}
					continue
				}
				body, _ := ioutil.ReadAll(resp.Body)
				resp.Body.Close()

				var ngrok = new(ngrokAPI)
				err = json.Unmarshal(body, &ngrok)
				if err != nil {
					chanErr <- err
					close(chanErr)
					return
				}
				if len(ngrok.Tunnels) == 0 {
					chanErr <- fmt.Errorf("Не удалось получить тунели ngrok")
					close(chanErr)
					return
				}
				for _, url := range ngrok.Tunnels {
					if strings.Index(strings.ToLower(url.PublicUrl), "https") >= 0 {
						result <- url.PublicUrl
						close(result)
						return
					}

				}
				chanErr <- fmt.Errorf("Не нашли https тунель ngrok")
				close(chanErr)
			}
		}(result, err)

		select {
		case e := <-err:
			return "", e
		case r := <-result:
			return r, nil
		}

	} else if net.WebhookURL != "" {
		return net.WebhookURL, nil
	} else {
		return "", fmt.Errorf("В настройках не определен блок Network или WebhookURL")
	}

	return "", nil
}

func saveFile(message *tgbotapi.Message, bot *tgbotapi.BotAPI) (err error) {
	downloadFilebyID := func(FileID string) {
		var file tgbotapi.File
		if file, err = bot.GetFile(tgbotapi.FileConfig{FileID}); err == nil {
			_, fileName := path.Split(file.FilePath)

			netU := new(n.NetUtility).Construct(file.Link(tel.Confs.BotToken), "", "")
			netU.Conf = tel.Confs
			err = netU.DownloadFile(path.Join("InFiles", fileName))
		}
	}

	if message.Video != nil {
		downloadFilebyID(message.Video.FileID)
	} else if message.Photo != nil {
		photos := *message.Photo
		// Последний элемент массива самого хорошего качества, берем его
		downloadFilebyID(photos[len(photos)-1].FileID)
	} else if message.Audio != nil {
		downloadFilebyID(message.Audio.FileID)
	} else if message.Voice != nil {
		downloadFilebyID(message.Voice.FileID)
	} else if message.Document != nil {
		downloadFilebyID(message.Document.FileID)
	} else {
		return fmt.Errorf("Не поддерживаемый тип данных")
	}

	return err
}

func NewBotAPI(WebhookURL string) *tgbotapi.BotAPI {

	bot, err := tgbotapi.NewBotAPIWithClient(tel.Confs.BotToken, n.GetHttpClient(tel.Confs))
	if err != nil {
		logrus.Errorf("Произошла ошибка при создании бота: %q", err)
		return nil
	}
	logrus.Debug("Устанавливаем хук на URL " + WebhookURL)

	//_, err = bot.SetWebhook(tgbotapi.NewWebhookWithCert(net.WebhookURL, "webhook_cert.pem"))
	_, err = bot.SetWebhook(tgbotapi.NewWebhook(WebhookURL))
	if err != nil {
		logrus.Errorf("Произошла ошибка при установки веб хука для бота: %q", err)
		return nil
	}

	//bot.Debug = true
	return bot
}

///////////////// RotateConf ////////////////////////////////////////////////////
func (w *RotateConf) LogDir() string {
	currentDir, _ := os.Getwd()
	return filepath.Join(currentDir, "Logs")
}
func (w *RotateConf) FormatDir() string {
	return "02.01.2006"
}
func (w *RotateConf) FormatFile() string {
	return "15"
}
func (w *RotateConf) TTLLogs() int {
	return 12
}
func (w *RotateConf) TimeRotate() int {
	return 1
}


// ДЛЯ ПАПЫ
/*
buildcfe - Собрать файлы расширений *.cfe
buildcf - Собрать файл конфигурации *.cf
buildanduploadcf - Собрать конфигурацию и отправить в менеджер сервиса
buildanduploadcfe - Собрать Файлы расширений и обновить в менеджер сервиса
setplanupdate - Запланировать обновление
getlistupdatestate - Получить список запланированных обновлений конфигураций
invokeupdate - Запуск задания jenkins для принудительного старта обработчиков обновления
ivokeupdateactualcfe - Запуск обновлений расширений через jenkins
deployextension - Отправка файла в МС, инкремент версии в ветки Dev, отправка задания на обновление в jenkins
disablezabbixmonitoring - Отключение zabbix мониторинга
charts - Графики
//cancel - Отмена текущего действия
*/

// go build -o "bot.exe" -ldflags "-s -w"