#include "b.h"

int x = 2;

int a() {
 return z;
}

int y; // without initializer y counts as an external global variable declaration.

int f() {
 int a = 5;
 return x + a;
}

int g() {
 f();
 x = 3;
 if (x) {
  return x;
 }
 return y;
}
