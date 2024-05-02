cmake_minimum_required(VERSION 3.5)
project(vegas)
include(CheckCCompilerFlag)

set(THIRD_PARTY_LIB_DIR "${vegas_SOURCE_DIR}/third_party/lib")
set(CMAKE_RUNTIME_OUTPUT_DIRECTORY "$<0:>${CMAKE_BINARY_DIR}/bin")
set(CMAKE_LIBRARY_OUTPUT_DIRECTORY "$<0:>/bin")
set(CMAKE_PREFIX_PATH, "${CMAKE_CURRENT_SOURCE_DIR}/third_party")

if(CMAKE_COMPILER_IS_GNUCXX)
    set(ADDL_LINK_CONFIG "-static-libstdc++")
    add_compile_options("-fpermissive")
endif()

if(CMAKE_BUILD_TYPE MATCHES Debug)
    add_definitions(-DDEBUG)
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -fsanitize=address")
    if (CMAKE_COMPILER_IS_GNUCXX)
        set(ADDL_LINK_CONFIG "${ADDL_LINK_CONFIG} -static-libasan")
    endif (CMAKE_COMPILER_IS_GNUCXX)
endif(CMAKE_BUILD_TYPE MATCHES Debug)

if(APPLE)
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -mmacosx-version-min=10.13 -framework Foundation -framework SystemConfiguration")
endif()

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_EXE_LINKER_FLAGS "-L${THIRD_PARTY_LIB_DIR} ${ADDL_LINK_CONFIG}")

set(SOURCE_FILES
        producer.hpp
        producer.cpp
        consumer.hpp
        consumer.cpp
        vegas.h
        vegas.cpp
)

set(TESTS_SOURCE
    producer_test.cpp
    consumer_test.cpp
)

set(THIRD_PARTY_LIBS third_party/lib)
set(THIRD_PARTY_INCLUDE third_party/include)

include_directories(${CMAKE_CURRENT_SOURCE_DIR})

if(UNIX AND NOT APPLE)
    set(LINUX TRUE)
endif()

if(LINUX)
    set(ADDITIONAL_LIBS idn)
endif()

find_package(Threads)
find_package(AWSSDK REQUIRED COMPONENTS kinesis monitoring sts)

add_library(${PROJECT_NAME} ${SOURCE_FILES})
target_include_directories(${PROJECT_NAME} SYSTEM PUBLIC ${THIRD_PARTY_INCLUDE})
target_link_libraries(${PROJECT_NAME} ${AWSSDK_LINK_LIBRARIES})
