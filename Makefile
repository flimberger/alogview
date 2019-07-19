PROG=	alogview
MAN=	${PROG}.1

# Locations
DESTDIR?=	/usr/local
BINDIR?=	/bin
MANDIR?=	/share/man/man1

# Tools
GOTOOL?=	go
INSTALL?=	install

all: ${PROG}

${PROG}: *.go
	${GOTOOL} build -o ${PROG}

install: ${PROG}
	install -C ${PROG} ${DESTDIR}${BINDIR}/${PROG}
	install -C ${MAN} ${DESTDIR}${MANDIR}/${MAN}

clean:
	rm -f ${PROG}

manlint: ${MAN}
	mandoc -Tlint ${MAN}

.PHONY: all install clean manlint
