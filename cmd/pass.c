#include <stdlib.h>
#include <termios.h>
#include <unistd.h>
#define LENPASSWD 30

static struct termios original;

char  *
GetPassword(int fildes)
{
	int    c, n;
	char  *passwd;
	passwd = (char *) malloc(LENPASSWD * sizeof (char));
	struct termios tmp;
	tcgetattr(fildes, &original);
	tmp = original;
	tmp.c_lflag &= ~ECHO;
	tcsetattr(fildes, TCSANOW, &tmp);
	n = read(fildes, passwd, LENPASSWD - 1);
	write(fildes, "\n", 1);
	for (c = passwd[n - 1]; n > 0 && (c == '\n' || c == '\r'); n--, c = passwd[n - 1]);
	passwd[n] = '\0';
	tcsetattr(fildes, TCSANOW, &original);
	return passwd;
}
