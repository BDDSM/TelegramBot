package telegram

import (
	cf "1C/Configuration"
	"1C/fresh"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	uuid "github.com/nu7hatch/gouuid"
	"github.com/sirupsen/logrus"
)

type BuilAndUploadCfe struct {
	BuildCfe

	freshConf *cf.FreshConf
}

func (B *BuilAndUploadCfe) ChoseMC(ChoseData string) {
	deferfunc := func() {
		if err := recover(); err != nil {
			Msg := fmt.Sprintf("Произошла ошибка при выполнении %q: %v", B.name, err)
			logrus.WithField("Каталог сохранения расширений", B.Ext.OutDir).Error(Msg)
			B.baseFinishMsg(Msg)
		} else {
			B.innerFinish()
		}
		B.outFinish()
	}

	//B.state = StateWork

	for _, conffresh := range Confs.FreshConf {
		if ChoseData == conffresh.Name {
			B.freshConf = conffresh
			break
		}
	}

	pool := 5
	B.outСhan = make(chan string, pool)

	go func() {
		wgLock := new(sync.WaitGroup)
		chError := make(chan error, pool)

		for c := range B.outСhan {
			wgLock.Add(1)
			fresh := new(fresh.Fresh)
			fresh.Conf = B.freshConf
			_, fileName := filepath.Split(c)

			B.bot.Send(tgbotapi.NewMessage(B.GetMessage().Chat.ID, fmt.Sprintf("Загружаем расширение %q в МС", fileName)))
			go fresh.RegExtension(wgLock, chError, c)
		}

		go func() {
			for err := range chError {
				msg := fmt.Sprintf("Произошла ошибка при выполнении %q: %v", B.name, err)
				logrus.Error(msg)
				B.baseFinishMsg(msg)
			}
		}()

		wgLock.Wait()
		close(chError)
		time.Sleep(time.Millisecond * 5)
		deferfunc()
	}()

	B.notInvokeInnerFinish = true                   // что бы не писалось сообщение о том, что расширения ожидают вас там-то
	B.StartInitialise(B.bot, B.update, B.outFinish) // вызываем родителя
}

func (B *BuilAndUploadCfe) StartInitialiseDesc(bot *tgbotapi.BotAPI, update *tgbotapi.Update, finish func()) {
	B.bot = bot
	B.update = update
	B.outFinish = finish

	msg := tgbotapi.NewMessage(B.GetMessage().Chat.ID, "Выберите менеджер сервиса для загрузки расширений")
	B.callback = make(map[string]func(), 0)
	Buttons := make([]map[string]interface{}, 0, 0)

	for _, conffresh := range Confs.FreshConf {
		UUID, _ := uuid.NewV4()
		Name := conffresh.Name // Обязательно через переменную, нужно для замыкания
		Buttons = append(Buttons, map[string]interface{}{
			"Alias": conffresh.Alias,
			"ID":    UUID.String(),
			"Invoke": func() {
				B.ChoseMC(Name)
			},
		})
	}

	B.CreateButtons(&msg, Buttons, 3, true)
	bot.Send(msg)
}

func (B *BuilAndUploadCfe) innerFinish() {
	B.baseFinishMsg("Готово!")
}
