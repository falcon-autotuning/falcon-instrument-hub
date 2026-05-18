cmake_minimum_required(VERSION 3.20)
project(
  falcon-instrument-hub-deps
  VERSION 1.0.0
  DESCRIPTION "Installs C library dependencies for falcon-instrument-hub via vcpkg"
  LANGUAGES CXX C
)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

find_package(falcon-core CONFIG REQUIRED)
