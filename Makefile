PROG=	alogview
MAN=	${PROG}.1

# Locations
DESTDIR?=	/usr/local
BINDIR?=	/bin
MANDIR?=	/share/man/man1

# Tools
GOTOOL?=	go
GOLINT?=	golint
INSTALL?=	install

all: ${PROG}

${PROG}: *.go
	${GOTOOL} build -o ${PROG}

install: ${PROG}
	install -C ${PROG} ${DESTDIR}${BINDIR}/${PROG}
	install -C ${MAN} ${DESTDIR}${MANDIR}/${MAN}

clean:
	rm -f ${PROG}

lint: golint manlint

golint:
	${GOLINT}

manlint:
	mandoc -Tlint ${MAN}

fmt:
	${GOTOOL} fmt

.PHONY: all install clean lint golint manlint fmt
