package telegram

import (
	cf "1C/Configuration"
	git "1C/Git"
	"1C/fresh"
	JK "1C/jenkins"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/sirupsen/logrus"
)

type DeployExtension struct {
	BuilAndUploadCfe
}

func (this *DeployExtension) Ini(bot *tgbotapi.BotAPI, update *tgbotapi.Update, finish func()) {
	this.bot = bot
	this.update = update
	this.outFinish = finish
	this.state = StateWork

	this.AfterUploadFresh = append(this.AfterUploadFresh, func(ext cf.IConfiguration) {
		logrus.Debugf("Инкрементируем версию расширения %q", ext.GetName())
		this.bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, "Меняем версию расшерения"))

		if err := ext.IncVersion(); err != nil {
			logrus.WithField("Расширение", ext.GetName()).Error(err)
		} else {
			this.CommitAndPush(ext.(*cf.Extension).ConfigurationFile)
		}

		msg := tgbotapi.NewMessage(this.GetMessage().Chat.ID, "Отправляем задание в jenkins, установить монопольно?")
		this.callback = make(map[string]func())
		Buttons := make([]map[string]interface{}, 0)
		this.appendButton(&Buttons, "Да", func() {
			if err := this.InvokeJobJenkins(ext, true); err == nil {
				bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, "Задание отправлено в jenkins"))
			}
		})
		this.appendButton(&Buttons, "Нет", func() {
			if err := this.InvokeJobJenkins(ext, false); err == nil {
				bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, "Задание отправлено в jenkins"))
			}
		})

		this.createButtons(&msg, Buttons, 3, true)
		bot.Send(msg)

	})

	this.AppendDescription(this.name)
	this.startInitialise_3(bot, update, finish)
}

func (this *DeployExtension) startInitialise_3(bot *tgbotapi.BotAPI, update *tgbotapi.Update, finish func()) {
	this.startInitialise_2(bot, update, finish) // метод предка
}

func (this *DeployExtension) innerFinish() {
	this.baseFinishMsg(fmt.Sprintf("Задание:\n%v\nГотово!", this.description))
	this.outFinish()
}

// GIT
func (this *DeployExtension) CommitAndPush(filePath string) {
	logrus.Debug("Коммитим версию в хранилище")

	g := new(git.Git)
	g.RepDir, _ = filepath.Split(filePath)
	branchName := "Dev"

	if g.BranchExist(branchName) {
		if err := g.CommitAndPush(branchName, filePath, "Автоинкремент версии"); err != nil {
			logrus.Errorf("Ошибка при коммите измененной версии: %v", err)
			this.bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, fmt.Sprintf("Ошибка при коммите измененной версии: %v", err)))
		}
	} else {
		logrus.WithField("Ветка", branchName).Error("Ветка не существует")
		this.bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, fmt.Sprintf("Ветка %q не существует", branchName)))
	}
}

//Jenkins
func (this *DeployExtension) InvokeJobJenkins(ext cf.IConfiguration, exclusive bool) (err error) {
	defer func() {
		if e := recover(); e != nil {
			logrus.Error(fmt.Sprintf("Произошла ошибка при выполнении %q: %v", this.name, e))
			this.innerFinish()
			err = fmt.Errorf("Ошибка при отправки в Jenkins: %v", e)
		} else {
			this.innerFinish()
		}
	}()

	fresh := new(fresh.Fresh)
	fresh.Conf = this.freshConf
	var Availablebases = []Bases{}
	var Allbases = []Bases{}
	this.JsonUnmarshal(fresh.GetAvailableDatabase(ext.GetName()), &Availablebases)
	this.JsonUnmarshal(fresh.GetDatabase(), &Allbases)

	var baseSM Bases
	var SMName string = "sm"
	errors := []error{}

	// Находим МС
	for _, DB := range Allbases {
		if strings.ToLower(DB.Name) == SMName {
			baseSM = DB
			break
		}
	}

	if baseSM.UUID == "" {
		errors = append(errors, fmt.Errorf("База %q не найдена", SMName))
	}
	if baseSM.UUID != "" && baseSM.UserName == "" {
		errors = append(errors, fmt.Errorf("У базы %q не задана учетная запись администратора", SMName))
	}
	if baseSM.UUID != "" && baseSM.UserPass == "" {
		errors = append(errors, fmt.Errorf("У базы %q не задан пароль учетной записи администратора", SMName))
	}
	if len(errors) > 0 {
		this.bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, "Произошли ошибки:"))
		for _, err := range errors {
			logrus.Error(err)
			this.bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, err.Error()))
		}
		return fmt.Errorf("Произошли ошибки, см. лог.")
	}

	result := map[string]int{
		"error":   0,
		"success": 0,
	}
	for _, DB := range Availablebases {
		jk := new(JK.Jenkins)
		jk.RootURL = Confs.Jenkins.URL
		jk.User = Confs.Jenkins.Login
		jk.Pass = Confs.Jenkins.Password
		jk.Token = Confs.Jenkins.UserToken
		err := jk.InvokeJob("update-cfe", map[string]string{
			"srv":        DB.Cluster.MainServer,
			"db":         DB.Name,
			"ras_srv":    DB.Cluster.RASServer,
			"ras_port":   fmt.Sprintf("%d", DB.Cluster.RASPort),
			"usr":        DB.UserName,
			"pwd":        DB.UserPass,
			"cfe_name":   ext.GetName(),
			"cfe_id":     ext.(*cf.Extension).GUID,
			"kill_users": strconv.FormatBool(exclusive),
			"SM_URL":     baseSM.URL,
			"SM_USR":     baseSM.UserName,
			"SM_PWD":     baseSM.UserPass,
		})
		if err != nil {
			logrus.Errorf("Ошибка при отправки задания update-cfe: %v", err)
			result["error"]++
		} else {
			result["success"]++
		}
	}

	msg := fmt.Sprintf("Задания успешно отправлены для %d баз", result["success"])
	if result["error"] > 0 {
		msg += fmt.Sprintf("\nДля %d баз произошла ошибка при отправки", result["error"])
	}
	this.bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, msg))

	// Отслеживаем статус
	go this.pullStatus()
	return nil
}

func (this *DeployExtension) pullStatus() {
	timer := time.NewTicker(time.Second * 10)
	for range timer.C {
		status := JK.GetJobStatus(Confs.Jenkins.URL, "update-cfe", Confs.Jenkins.Login, Confs.Jenkins.Password)
		switch status {
		case JK.Error:
			this.bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, "Выполнение задания \"update-cfe\" завершилось с ошибкой"))
			timer.Stop()
		case JK.Done:
			this.bot.Send(tgbotapi.NewMessage(this.GetMessage().Chat.ID, "Задания \"update-cfe\" выполнено"))
			timer.Stop()
		}
	}
}