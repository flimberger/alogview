PROG=	alogview
MAN=	${PROG}.1

# Locations
DESTDIR?=	/usr/local
BINDIR?=	/bin
MANDIR?=	/share/man/man1

# Tools
GOTOOL?=	go
INSTALL?=	install

all: ${PROG} test

${PROG}: *.go
	${GOTOOL} build -o ${PROG}

install: ${PROG}
	install -C ${PROG} ${DESTDIR}${BINDIR}/${PROG}
	install -C ${MAN} ${DESTDIR}${MANDIR}/${MAN}

clean:
	rm -f ${PROG}

lint: golint manlint

golint:
	gofmt -l .
	go vet

manlint:
	mandoc -Tlint ${MAN}

fmt:
	${GOTOOL} fmt

test:
	${GOTOOL} test

.PHONY: all install clean lint golint manlint fmt test
