.PHONY: build check run
build:
	bash scripts/build.sh
check:
	bash scripts/check.sh
run: build
	build/NativeJet.app/Contents/MacOS/NativeJet --dev
