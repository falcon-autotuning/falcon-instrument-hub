vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/falcon-measurement-lib
    REF v${VERSION}
    SHA512 8a6f44ad2732a98f7543f48647d9a6354b47a409ddaa74690228f99aa392b6a7ed7e94908e14a787e009b8fbcfccbd2f70db6af414531c214bddd497fe521d35
)

# Set up Lua venv paths inside the package
set(LUA_VENV "${CURRENT_BUILDTREES_DIR}/lua-venv")
set(LUA_VENV_BIN "${LUA_VENV}/bin")
set(LUA_VENV_TREE "${LUA_VENV}/tree")
file(MAKE_DIRECTORY "${LUA_VENV}")
file(MAKE_DIRECTORY "${LUA_VENV_BIN}")
file(MAKE_DIRECTORY "${LUA_VENV_TREE}")


# Create venv Lua launcher
find_program(LUA_EXECUTABLE NAMES lua5.4 lua)
file(WRITE "${LUA_VENV_BIN}/lua"
   "#!/bin/sh\nLUA_PATH=\"${LUA_VENV_TREE}/share/lua/5.4/?.lua;${LUA_VENV_TREE}/share/lua/5.4/?/init.lua;;\" LUA_CPATH=\"${LUA_VENV_TREE}/lib/lua/5.4/?.so;;\" exec ${LUA_EXECUTABLE} \"\$@\"\n")
execute_process(COMMAND chmod +x "${LUA_VENV_BIN}/lua")

# Create venv luarocks launcher
find_program(LUAROCKS_EXECUTABLE NAMES luarocks luarocks5.4)
file(WRITE "${LUA_VENV_BIN}/luarocks"
"#!/bin/sh\nLUAROCKS_TREE=\"${LUA_VENV_TREE}\" exec luarocks --tree=\"${LUA_VENV_TREE}\" \"\$@\"\n")
execute_process(COMMAND chmod +x "${LUA_VENV_BIN}/luarocks")

# Install Lua dependencies into the venv
foreach(MODULE dkjson luafilesystem compat53 argparse)
  vcpkg_execute_required_process(
        COMMAND ${LUAROCKS_EXECUTABLE} --lua-version=5.4 --tree=${LUA_VENV_TREE} install ${MODULE}
        WORKING_DIRECTORY "${SOURCE_PATH}"
        LOGNAME luarocks-install-${MODULE}
    )
  vcpkg_execute_required_process(
      COMMAND ${LUAROCKS_EXECUTABLE} --lua-version=5.4 --tree=${LUA_VENV_TREE} show ${MODULE}
      WORKING_DIRECTORY "${SOURCE_PATH}"
      LOGNAME luarocks-show-${MODULE}
  )
endforeach()

# Run the generator using the venv Lua
set(OUT_DIR "${SOURCE_PATH}/generated/lua")
file(MAKE_DIRECTORY "${OUT_DIR}")

vcpkg_execute_required_process(
    COMMAND sh -c
        "LUA_PATH='${LUA_VENV_TREE}/share/lua/5.4/?.lua;${LUA_VENV_TREE}/share/lua/5.4/?/init.lua;;' LUA_CPATH='${LUA_VENV_TREE}/lib/lua/5.4/?.so;;' '${LUA_VENV_BIN}/lua' ./generator/gen_from_schemas.lua ./schemas/lib ./schemas/scripts ./src/lua '${OUT_DIR}'"
    WORKING_DIRECTORY "${SOURCE_PATH}"
    LOGNAME lua-generate
)

# Only install the falcon_measurement_lib directory from generated/lua
file(INSTALL
    "${OUT_DIR}/lua/falcon_measurement_lib"
    DESTINATION "${CURRENT_PACKAGES_DIR}/lib/lua/5.4"
)

# Copy all Lua modules from the venv into lib/lua/5.4
file(GLOB LUA_MODULES "${LUA_VENV_TREE}/share/lua/5.4/*")
file(GLOB LUA_SO "${LUA_VENV_TREE}/lib/lua/5.4/*")

file(COPY ${LUA_MODULES} DESTINATION "${CURRENT_PACKAGES_DIR}/lib/lua/5.4")
file(COPY ${LUA_SO} DESTINATION "${CURRENT_PACKAGES_DIR}/lib/lua/5.4")

# License
file(INSTALL "${SOURCE_PATH}/LICENSE" DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}" RENAME copyright)
