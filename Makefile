# Makefile de PokerFight para participantes. Con Go alcanza: cada comando de aquí
# es una línea de `go build` y de ./bin/..., que puedes correr a mano si no tienes make.
BIN := bin

.PHONY: ayuda build torneo-local mesa entrenar-darwin limpiar

ayuda: ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

build: ## Compila la arena y los bots en ./bin
	go build -o $(BIN)/ ./cmd/... ./bots/...

torneo-local: build ## Un torneo de prueba entre los bots de ejemplo (solo Go)
	$(BIN)/arena correr -config torneo-go.json

mesa: build ## Una mesa en :9000 para conectarle bots a mano, con el plazo de la competencia
	$(BIN)/mesa -addr :9000 -jugadores 6 -min-jugadores 3 -rondas 200 -timeout-ms 5000

entrenar-darwin: ## Reentrena el genoma de Gato Darwin (reemplaza bots/darwin/genoma.json) y recompila
	go run ./bots/darwin/cmd/entrenar-darwin
	go build -o $(BIN)/ ./bots/darwin

limpiar: ## Borra binarios y resultados
	rm -rf $(BIN) resultados
