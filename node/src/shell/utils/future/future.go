package future

// import "log"

func Async(runnable func(), retriable bool) {
	if retriable {
		go func ()  {
			retriableFunc := func () {
				// defer func ()  {
				// 	if err := recover(); err != nil {
				// 		log.Println(err)
				// 	}
				// }()
				runnable()
			}
			for {
				retriableFunc()
			}
		}()
	} else {
		go func ()  {
			// defer func ()  {
			// 	if err := recover(); err != nil {
			// 		log.Println(err)
			// 	}
			// }()
			runnable()
		}()
	}
}